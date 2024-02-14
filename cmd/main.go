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
	"bytes"
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

	"github.com/bucket-sailor/picoceph/data"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))

	logger.Info("Writing ceph.conf")

	fsid := uuid.New().String()

	if err := writeCephConf(fsid); err != nil {
		logger.Error("Could not write ceph.conf", "error", err)
		os.Exit(1)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logger.Info("Configuring monitor")

		if err := configureMonitor(ctx, fsid); err != nil {
			return fmt.Errorf("could not configure monitor: %w", err)
		}

		logger.Info("Starting monitor")

		cmd := exec.CommandContext(ctx, "ceph-mon", "-f", "-i", "a")
		if out, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(err.Error(), "signal: killed") {
				return nil
			}

			return fmt.Errorf("could not start monitor: %w: %s", err, out)
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
		if out, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(err.Error(), "signal: killed") {
				return nil
			}

			return fmt.Errorf("could not start manager: %w: %s", err, out)
		}

		return nil
	})

	g.Go(func() error {
		logger.Info("Creating OSD device")

		if err := createOSDDevice(ctx); err != nil {
			return fmt.Errorf("could not create OSD device: %w", err)
		}

		logger.Info("Preparing OSD")

		cmd := exec.CommandContext(ctx, "ceph-volume", "lvm", "create", "--no-systemd", "--data", "ceph-vg/osd-0", "--osd-id", "0")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("could not prepare OSD device: %w: %s", err, out)
		}

		logger.Info("Starting OSD")

		cmd = exec.CommandContext(ctx, "ceph-osd", "-f", "--id", "0")
		if out, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(err.Error(), "signal: killed") {
				return nil
			}

			return fmt.Errorf("could not start OSD: %w: %s", err, out)
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
		if out, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(err.Error(), "signal: killed") {
				return nil
			}

			return fmt.Errorf("could not start RADOS Gateway: %w: %s", err, out)
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

	radosgwKeyring, err := os.Create("/var/lib/ceph/radosgw/ceph-radosgw.gateway/keyring")
	if err != nil {
		return fmt.Errorf("could not create keyring: %w", err)
	}
	defer radosgwKeyring.Close()

	cephCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cephCtx, "ceph", "auth", "get-or-create", "client.radosgw.gateway", "osd", "allow rwx", "mon", "allow rw")
	cmd.Stdout = radosgwKeyring

	var out bytes.Buffer
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, out.String())
	}

	cephUserUid, cephGroupGid, err := cephUser()
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	if err := chownRecursive("/var/lib/ceph/radosgw/ceph-radosgw.gateway", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func configureManager(ctx context.Context) error {
	if err := os.MkdirAll("/var/lib/ceph/mgr/ceph-a", 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
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

	var out bytes.Buffer
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, out.String())
	}

	cephUserUid, cephGroupGid, err := cephUser()
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	if err := chownRecursive("/var/lib/ceph/mgr/ceph-a", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func configureMonitor(ctx context.Context, fsid string) error {
	if err := os.MkdirAll("/var/lib/ceph/mon/ceph-a", 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	cmd := exec.CommandContext(ctx, "ceph-authtool", "--create-keyring", "/tmp/ceph.mon.keyring", "--gen-key", "-n", "mon.", "--cap", "mon", "allow *")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, out)
	}

	cmd = exec.CommandContext(ctx, "ceph-authtool", "--create-keyring", "/etc/ceph/ceph.client.admin.keyring", "--gen-key", "-n", "client.admin", "--cap", "mon", "allow *", "--cap", "osd", "allow *", "--cap", "mds", "allow *", "--cap", "mgr", "allow *")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, out)
	}

	cmd = exec.CommandContext(ctx, "ceph-authtool", "--create-keyring", "/var/lib/ceph/bootstrap-osd/ceph.keyring", "--gen-key", "-n", "client.bootstrap-osd", "--cap", "mon", "profile bootstrap-osd", "--cap", "mgr", "allow r")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, out)
	}

	cmd = exec.CommandContext(ctx, "ceph-authtool", "/tmp/ceph.mon.keyring", "--import-keyring", "/etc/ceph/ceph.client.admin.keyring")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not import keyring: %w: %s", err, out)
	}

	cmd = exec.CommandContext(ctx, "ceph-authtool", "/tmp/ceph.mon.keyring", "--import-keyring", "/var/lib/ceph/bootstrap-osd/ceph.keyring")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not import keyring: %w: %s", err, out)
	}

	cmd = exec.CommandContext(ctx, "monmaptool", "--create", "--addv", "a", "[v2:127.0.0.1:3300,v1:127.0.0.1:6789]", "--fsid", fsid, "/tmp/monmap")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create monmap: %w: %s", err, out)
	}

	cmd = exec.CommandContext(ctx, "ceph-mon", "--mkfs", "-i", "a", "--monmap", "/tmp/monmap", "--keyring", "/tmp/ceph.mon.keyring")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create monitor: %w: %s", err, out)
	}

	cephUserUid, cephGroupGid, err := cephUser()
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	if err := chownRecursive("/etc/ceph", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	if err := os.Chown("/var/lib/ceph/mon/ceph-a", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func writeCephConf(fsid string) error {
	cephConf, err := os.Create("/etc/ceph/ceph.conf")
	if err != nil {
		return fmt.Errorf("could not create ceph.conf: %w", err)
	}
	defer cephConf.Close()

	tmpl := data.GetCephConfTmpl()

	if err := tmpl.Execute(cephConf, struct {
		FSID string
	}{
		FSID: fsid,
	}); err != nil {
		return fmt.Errorf("could not execute ceph.conf template: %w", err)
	}

	cephUserUid, cephGroupGid, err := cephUser()
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	if err := os.Chown("/etc/ceph/ceph.conf", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func createOSDDevice(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "/usr/sbin/dmsetup", "remove", "-v", "ceph--vg-osd--0")
	_ = cmd.Run()

	if err := os.RemoveAll("/dev/ceph-vg"); err != nil {
		return fmt.Errorf("could not remove directory: %w", err)
	}

	if err := os.MkdirAll("/var/lib/ceph/disk", 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	// Create a qemu image.
	cmd = exec.CommandContext(ctx, "qemu-img", "create", "-f", "qcow2", "/var/lib/ceph/disk/osd-0.qcow2", "10G")
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

func cephUser() (int, int, error) {
	cephUser, err := user.Lookup("ceph")
	if err != nil {
		return -1, -1, fmt.Errorf("could not get ceph user: %w", err)
	}

	cephUserUid, err := strconv.Atoi(cephUser.Uid)
	if err != nil {
		return -1, -1, fmt.Errorf("could not convert ceph user uid: %w", err)
	}

	cephGroup, err := user.LookupGroup("ceph")
	if err != nil {
		return -1, -1, fmt.Errorf("could not get ceph group: %w", err)
	}

	cephGroupGid, err := strconv.Atoi(cephGroup.Gid)
	if err != nil {
		return -1, -1, fmt.Errorf("could not convert ceph group gid: %w", err)
	}

	return cephUserUid, cephGroupGid, nil
}

func chownRecursive(path string, uid, gid int) error {
	if err := os.Chown(path, uid, gid); err != nil {
		return err
	}

	return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		return os.Chown(path, uid, gid)
	})
}
