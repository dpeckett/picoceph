// SPDX-License-Identifier: MPL-2.0
/*
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package nbd

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Setup ensures that the nbd kernel module is loaded and that the kernel supports nbd.
func Setup(ctx context.Context) error {
	// Load the nbd kernel module (if not already loaded or built-in).
	cmd := exec.CommandContext(ctx, "/sbin/modprobe", "nbd")
	_ = cmd.Run()

	// Do we have support for nbd?
	if _, err := os.Stat("/sys/block/nbd0"); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("your kernel does not support nbd")
	}

	return nil
}

// NextFreeDevice returns the path to the next free NBD device.
func NextFreeDevice() (string, error) {
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
