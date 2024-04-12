// SPDX-License-Identifier: MPL-2.0
/*
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package monitor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bucket-sailor/picoceph/internal/ceph"
	"github.com/bucket-sailor/picoceph/internal/util"
)

type Monitor struct {
	fsid string
}

func New(fsid string) ceph.Component {
	return &Monitor{
		fsid: fsid,
	}
}

func (m *Monitor) Name() string {
	return "monitor"
}

func (m *Monitor) Configure(ctx context.Context) error {
	if err := os.MkdirAll("/var/lib/ceph/mon/ceph-a", 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	cmd := exec.CommandContext(ctx, "ceph-authtool", "--create-keyring", "/tmp/ceph.mon.keyring", "--gen-key", "-n", "mon.", "--cap", "mon", "allow *")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "ceph-authtool", "--create-keyring", "/etc/ceph/ceph.client.admin.keyring", "--gen-key", "-n", "client.admin", "--cap", "mon", "allow *", "--cap", "osd", "allow *", "--cap", "mds", "allow *", "--cap", "mgr", "allow *")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "ceph-authtool", "--create-keyring", "/var/lib/ceph/bootstrap-osd/ceph.keyring", "--gen-key", "-n", "client.bootstrap-osd", "--cap", "mon", "profile bootstrap-osd", "--cap", "mgr", "allow r")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "ceph-authtool", "/tmp/ceph.mon.keyring", "--import-keyring", "/etc/ceph/ceph.client.admin.keyring")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not import keyring: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "ceph-authtool", "/tmp/ceph.mon.keyring", "--import-keyring", "/var/lib/ceph/bootstrap-osd/ceph.keyring")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not import keyring: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "monmaptool", "--create", "--addv", "a", "[v2:127.0.0.1:3300,v1:127.0.0.1:6789]", "--fsid", m.fsid, "/tmp/monmap")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create monmap: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "ceph-mon", "--mkfs", "-i", "a", "--monmap", "/tmp/monmap", "--keyring", "/tmp/ceph.mon.keyring")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create monitor: %w: %s", err, string(out))
	}

	cephUserUid, cephGroupGid, err := ceph.User()
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	if err := util.ChownRecursive("/etc/ceph", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	if err := os.Chown("/var/lib/ceph/mon/ceph-a", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func (m *Monitor) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ceph-mon", "-f", "-i", "a")
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			return nil
		}

		return fmt.Errorf("could not start monitor: %w: %s", err, string(out))
	}

	return nil
}
