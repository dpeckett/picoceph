/* SPDX-License-Identifier: MPL-2.0
 *
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logger.Info("Starting monitor")

		cmd := exec.CommandContext(ctx, "ceph-mon", "-f", "-i", "a")
		if err := cmd.Run(); err != nil {
			if strings.Contains(err.Error(), "signal: killed") {
				return nil
			}

			return fmt.Errorf("could not start monitor: %w", err)
		}

		return nil
	})

	g.Go(func() error {
		logger.Info("Configuring manager")

		if err := configureManager(ctx); err != nil {
			return fmt.Errorf("could not configure manager: %w", err)
		}

		logger.Info("Starting manager")

		cmd := exec.CommandContext(ctx, "ceph-mgr", "-f", "-i", "a")
		if err := cmd.Run(); err != nil {
			if strings.Contains(err.Error(), "signal: killed") {
				return nil
			}

			return fmt.Errorf("could not start manager: %w", err)
		}

		return nil
	})

	g.Go(func() error {
		logger.Info("Creating OSD device")

		if err := createOSDDevice(ctx); err != nil {
			return fmt.Errorf("could not create OSD device: %w", err)
		}

		logger.Info("Preparing OSD")

		cmd := exec.CommandContext(ctx, "ceph-volume", "lvm", "create", "--data", "ceph-vg/osd-0", "--osd-id", "0")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("could not prepare OSD: %w", err)
		}

		logger.Info("Starting OSD")

		cmd = exec.CommandContext(ctx, "ceph-osd", "-f", "--id", "0")
		if err := cmd.Run(); err != nil {
			if strings.Contains(err.Error(), "signal: killed") {
				return nil
			}

			return fmt.Errorf("could not start OSD: %w", err)
		}

		return nil
	})

	g.Go(func() error {
		logger.Info("Configuring RADOS Gateway")

		if err := configureRADOSGateway(ctx); err != nil {
			return fmt.Errorf("could not configure RADOS Gateway: %w", err)
		}

		logger.Info("Starting RADOS Gateway")

		cmd := exec.CommandContext(ctx, "radosgw", "-f", "-n", "client.radosgw.gateway")
		if err := cmd.Run(); err != nil {
			if strings.Contains(err.Error(), "signal: killed") {
				return nil
			}

			return fmt.Errorf("could not start RADOS Gateway: %w", err)
		}

		return nil
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("Could not run picoceph", "error", err)

		os.Exit(1)
	}
}

func configureRADOSGateway(ctx context.Context) error {
	if err := os.MkdirAll("/var/lib/ceph/radosgw/ceph-radosgw.gateway", 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	// Get the ceph user uid
	cephUser, err := user.Lookup("ceph")
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	cephUserUid, err := strconv.Atoi(cephUser.Uid)
	if err != nil {
		return fmt.Errorf("could not convert ceph user uid: %w", err)
	}

	cephGroup, err := user.LookupGroup("ceph")
	if err != nil {
		return fmt.Errorf("could not get ceph group: %w", err)
	}

	cephGroupGid, err := strconv.Atoi(cephGroup.Gid)
	if err != nil {
		return fmt.Errorf("could not convert ceph group gid: %w", err)
	}

	if err := os.Chown("/var/lib/ceph/radosgw/ceph-radosgw.gateway", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	radosgwKeyring, err := os.Create("/var/lib/ceph/radosgw/ceph-radosgw.gateway/keyring")
	if err != nil {
		return fmt.Errorf("could not create keyring: %w", err)
	}
	defer radosgwKeyring.Close()

	cephCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cephCtx, "ceph", "auth", "get-or-create", "client.radosgw.gateway", "osd", "allow rwx", "mon", "allow rw")
	cmd.Stdout = radosgwKeyring

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not create keyring: %w", err)
	}

	if err := os.Chown("/var/lib/ceph/radosgw/ceph-radosgw.gateway/keyring", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func configureManager(ctx context.Context) error {
	if err := os.MkdirAll("/var/lib/ceph/mgr/ceph-a", 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	// Get the ceph user uid
	cephUser, err := user.Lookup("ceph")
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	cephUserUid, err := strconv.Atoi(cephUser.Uid)
	if err != nil {
		return fmt.Errorf("could not convert ceph user uid: %w", err)
	}

	cephGroup, err := user.LookupGroup("ceph")
	if err != nil {
		return fmt.Errorf("could not get ceph group: %w", err)
	}

	cephGroupGid, err := strconv.Atoi(cephGroup.Gid)
	if err != nil {
		return fmt.Errorf("could not convert ceph group gid: %w", err)
	}

	if err := os.Chown("/var/lib/ceph/mgr/ceph-a", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	mgrKeyring, err := os.Create("/var/lib/ceph/mgr/ceph-a/keyring")
	if err != nil {
		return fmt.Errorf("could not create keyring: %w", err)
	}
	defer mgrKeyring.Close()

	cephCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cephCtx, "ceph", "auth", "get-or-create", "mgr.a", "mon", "allow profile mgr", "osd", "allow *", "mds", "allow *")
	cmd.Stdout = mgrKeyring

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not create keyring: %w", err)
	}

	if err := os.Chown("/var/lib/ceph/mgr/ceph-a/keyring", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func createOSDDevice(ctx context.Context) error {
	devMapperDevicePath := "/dev/mapper/ceph--vg-osd--0"
	if _, err := os.Stat(devMapperDevicePath); err == nil {
		cmd := exec.CommandContext(ctx, "/usr/sbin/dmsetup", "remove", "-v", devMapperDevicePath)
		_ = cmd.Run()

		if err := os.RemoveAll(devMapperDevicePath); err != nil {
			return fmt.Errorf("could not remove directory: %w", err)
		}

		if err := os.RemoveAll("/dev/ceph-vg"); err != nil {
			return fmt.Errorf("could not remove directory: %w", err)
		}
	}

	if err := os.MkdirAll("/var/lib/ceph/disk", 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	// Create a qemu image.
	cmd := exec.CommandContext(ctx, "qemu-img", "create", "-f", "qcow2", "/var/lib/ceph/disk/osd-0.qcow2", "10G")
	if output, err := cmd.Output(); err != nil {
		return fmt.Errorf("could not create qemu image: %w: %s", err, output)
	}

	// Mount the image using nbd.
	cmd = exec.CommandContext(ctx, "/sbin/modprobe", "nbd")
	if output, err := cmd.Output(); err != nil {
		return fmt.Errorf("could not load nbd module: %w: %s", err, output)
	}

	// Find the next free nbd device.
	nbdDevicePath, err := findNextFreeNBDDevice()
	if err != nil {
		return fmt.Errorf("could not find free nbd device: %w", err)
	}

	cmd = exec.CommandContext(ctx, "qemu-nbd", "--connect="+nbdDevicePath, "/var/lib/ceph/disk/osd-0.qcow2")
	if output, err := cmd.Output(); err != nil {
		return fmt.Errorf("could not mount qemu image: %w: %s", err, output)
	}

	cmd = exec.CommandContext(ctx, "pvcreate", nbdDevicePath)
	cmd.Env = append(os.Environ(), "DM_DISABLE_UDEV=1")
	if output, err := cmd.Output(); err != nil {
		return fmt.Errorf("could not create physical volume: %w: %s", err, output)
	}

	cmd = exec.CommandContext(ctx, "vgcreate", "ceph-vg", nbdDevicePath)
	cmd.Env = append(os.Environ(), "DM_DISABLE_UDEV=1")
	if output, err := cmd.Output(); err != nil {
		return fmt.Errorf("could not create volume group: %w: %s", err, output)
	}

	cmd = exec.CommandContext(ctx, "lvcreate", "-l", "100%FREE", "-n", "osd-0", "ceph-vg")
	cmd.Env = append(os.Environ(), "DM_DISABLE_UDEV=1")
	if output, err := cmd.Output(); err != nil {
		return fmt.Errorf("could not create logical volume: %w: %s", err, output)
	}

	return nil
}

func findNextFreeNBDDevice() (string, error) {
	dir, err := os.Open("/sys/block")
	if err != nil {
		return "", fmt.Errorf("could not open /sys/block: %w", err)
	}
	defer dir.Close()

	devices, err := dir.Readdirnames(-1)
	if err != nil {
		return "", fmt.Errorf("could not read /sys/block: %w", err)
	}

	var availableNBDDevices []string
	for _, dev := range devices {
		if strings.HasPrefix(dev, "nbd") {
			if _, err := os.ReadFile(filepath.Join("/sys/block", dev, "pid")); errors.Is(err, os.ErrNotExist) {
				availableNBDDevices = append(availableNBDDevices, dev)
			}
		}
	}

	if len(availableNBDDevices) == 0 {
		return "", fmt.Errorf("no free nbd devices found")
	}

	return filepath.Join("/dev/", availableNBDDevices[rand.Intn(len(availableNBDDevices))]), nil
}
