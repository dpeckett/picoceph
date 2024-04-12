// SPDX-License-Identifier: MPL-2.0
/*
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package osd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bucket-sailor/picoceph/internal/ceph"
	"github.com/bucket-sailor/picoceph/internal/nbd"
	"github.com/nxadm/tail"
)

type OSD struct {
	id string
}

func New(id string) ceph.Component {
	return &OSD{
		id: id,
	}
}

func (osd *OSD) Name() string {
	return fmt.Sprintf("osd (osd.%s)", osd.id)
}

func (osd *OSD) Configure(ctx context.Context) error {
	if err := osd.createDevice(ctx); err != nil {
		return fmt.Errorf("could not create OSD device: %w", err)
	}

	// Prepare the OSD device.
	cmd := exec.CommandContext(ctx, "ceph-volume", "lvm", "create", "--no-systemd", "--data", fmt.Sprintf("ceph-vg-%s/osd", osd.id), "--osd-id", osd.id)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not prepare OSD device: %w: %s", err, string(out))
	}

	return nil
}

func (osd *OSD) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ceph-osd", "-f", "--id", osd.id)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			return nil
		}

		return fmt.Errorf("could not start OSD: %w: %s", err, string(out))
	}

	return nil
}

// createDevice creates a new NBD block device for the OSD.
func (osd *OSD) createDevice(ctx context.Context) error {
	// Clean up any orphaned device nodes from previous runs.
	cmd := exec.CommandContext(ctx, "/usr/sbin/dmsetup", "remove", "-v", fmt.Sprintf("ceph--vg--%s-osd", osd.id))
	_ = cmd.Run()

	if err := os.RemoveAll("/dev/ceph-vg-" + osd.id); err != nil {
		return fmt.Errorf("could not remove directory: %w", err)
	}

	if err := os.MkdirAll("/var/lib/ceph/disk", 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	// Create a qemu image.
	cmd = exec.CommandContext(ctx, "qemu-img", "create", "-f", "qcow2", fmt.Sprintf("/var/lib/ceph/disk/osd-%s.qcow2", osd.id), "10G")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create qemu image: %w: %s", err, string(out))
	}

	// Load the nbd kernel module (if not already loaded or built-in).
	if err := nbd.Setup(ctx); err != nil {
		// TODO: maybe we can fall back to using a loop device?
		return fmt.Errorf("could not setup nbd: %w", err)
	}

	// Find the next free nbd device.
	nbdDevicePath, err := nbd.NextFreeDevice()
	if err != nil {
		return fmt.Errorf("could not find free nbd device: %w", err)
	}

	// Mount the image using nbd.
	cmd = exec.CommandContext(ctx, "qemu-nbd", "--connect="+nbdDevicePath, fmt.Sprintf("/var/lib/ceph/disk/osd-%s.qcow2", osd.id))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not mount qemu image: %w: %s", err, string(out))
	}

	// Set up the image for use with LVM.
	cmd = exec.CommandContext(ctx, "pvcreate", nbdDevicePath)
	cmd.Env = append(os.Environ(), "DM_DISABLE_UDEV=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create physical volume: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "vgcreate", "ceph-vg-"+osd.id, nbdDevicePath)
	cmd.Env = append(os.Environ(), "DM_DISABLE_UDEV=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create volume group: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "lvcreate", "-l", "100%FREE", "-n", "osd", "ceph-vg-"+osd.id)
	cmd.Env = append(os.Environ(), "DM_DISABLE_UDEV=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create logical volume: %w: %s", err, string(out))
	}

	return nil
}

func (osd *OSD) Logs() (*tail.Tail, error) {
	return tail.TailFile(
		fmt.Sprintf("/var/log/ceph/ceph-osd.%s.log", osd.id),
		tail.Config{Follow: true, ReOpen: true},
	)
}
