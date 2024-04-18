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

	"github.com/dpeckett/picoceph/internal/ceph"
	"github.com/dpeckett/picoceph/internal/util"
	"github.com/nxadm/tail"
)

type Monitor struct {
	id   string
	fsid string
}

func New(id, fsid string) ceph.Component {
	return &Monitor{
		id:   id,
		fsid: fsid,
	}
}

func (mon *Monitor) Name() string {
	return fmt.Sprintf("monitor (mon.%s)", mon.id)
}

func (mon *Monitor) Configure(ctx context.Context) error {
	if _, err := os.Stat("/etc/ceph/ceph.client.admin.keyring"); os.IsNotExist(err) {
		cmd := exec.CommandContext(ctx, "ceph-authtool", "--create-keyring", "/etc/ceph/ceph.client.admin.keyring", "--gen-key", "-n", "client.admin", "--cap", "mon", "allow *", "--cap", "osd", "allow *", "--cap", "mds", "allow *", "--cap", "mgr", "allow *")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("could not create keyring: %w: %s", err, string(out))
		}
	}

	if _, err := os.Stat("/var/lib/ceph/bootstrap-osd/ceph.keyring"); os.IsNotExist(err) {
		cmd := exec.CommandContext(ctx, "ceph-authtool", "--create-keyring", "/var/lib/ceph/bootstrap-osd/ceph.keyring", "--gen-key", "-n", "client.bootstrap-osd", "--cap", "mon", "profile bootstrap-osd", "--cap", "mgr", "allow r")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("could not create keyring: %w: %s", err, string(out))
		}
	}

	keyRingPath := fmt.Sprintf("/tmp/ceph.mon.%s.keyring", mon.id)

	cmd := exec.CommandContext(ctx, "ceph-authtool", "--create-keyring", keyRingPath, "--gen-key", "-n", "mon.", "--cap", "mon", "allow *")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "ceph-authtool", keyRingPath, "--import-keyring", "/etc/ceph/ceph.client.admin.keyring")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not import keyring: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "ceph-authtool", keyRingPath, "--import-keyring", "/var/lib/ceph/bootstrap-osd/ceph.keyring")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not import keyring: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "monmaptool", "--create", "--addv", mon.id, "[v2:127.0.0.1:3300,v1:127.0.0.1:6789]", "--fsid", mon.fsid, "/tmp/monmap-"+mon.id)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create monmap: %w: %s", err, string(out))
	}

	if err := os.MkdirAll("/var/lib/ceph/mon/ceph-"+mon.id, 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	cmd = exec.CommandContext(ctx, "ceph-mon", "--mkfs", "-i", mon.id, "--monmap", "/tmp/monmap-"+mon.id, "--keyring", keyRingPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create monitor: %w: %s", err, string(out))
	}

	// Delete the temporary keyring and monmap.
	if err := os.Remove(keyRingPath); err != nil {
		return fmt.Errorf("could not delete keyring: %w", err)
	}

	if err := os.RemoveAll("/tmp/monmap-" + mon.id); err != nil {
		return fmt.Errorf("could not delete temporary monmap: %w", err)
	}

	cephUserUid, cephGroupGid, err := ceph.User()
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	if err := util.ChownRecursive("/etc/ceph", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	if err := os.Chown("/var/lib/ceph/mon/ceph-"+mon.id, cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func (mon *Monitor) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ceph-mon", "-f", "-i", mon.id)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			return nil
		}

		return fmt.Errorf("could not start monitor: %w: %s", err, string(out))
	}

	return nil
}

func (mon *Monitor) Logs() (*tail.Tail, error) {
	return tail.TailFile(
		fmt.Sprintf("/var/log/ceph/ceph-mon.%s.log", mon.id),
		tail.Config{Follow: true, ReOpen: true},
	)
}
