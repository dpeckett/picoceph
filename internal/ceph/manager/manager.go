// SPDX-License-Identifier: MPL-2.0
/*
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package manager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bucket-sailor/picoceph/internal/ceph"
	"github.com/bucket-sailor/picoceph/internal/util"
	"github.com/nxadm/tail"
)

type Manager struct {
	id string
}

func New(id string) ceph.Component {
	return &Manager{
		id: id,
	}
}

func (mgr *Manager) Name() string {
	return fmt.Sprintf("manager (mgr.%s)", mgr.id)
}

func (mgr *Manager) Configure(ctx context.Context) error {
	if err := os.MkdirAll("/var/lib/ceph/mgr/ceph-"+mgr.id, 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	mgrKeyring, err := os.Create(fmt.Sprintf("/var/lib/ceph/mgr/ceph-%s/keyring", mgr.id))
	if err != nil {
		return fmt.Errorf("could not create keyring: %w", err)
	}
	defer mgrKeyring.Close()

	// Don't block forever if ceph does not come up.
	cephCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(cephCtx, "ceph", "auth", "get-or-create", fmt.Sprintf("mgr.%s", mgr.id), "mon", "allow profile mgr", "osd", "allow *", "mds", "allow *")
	cmd.Stdout = mgrKeyring

	var out strings.Builder
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, out.String())
	}

	cephUserUid, cephGroupGid, err := ceph.User()
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	if err := util.ChownRecursive("/var/lib/ceph/mgr/ceph-"+mgr.id, cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func (mgr *Manager) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ceph-mgr", "-f", "-i", mgr.id)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			return nil
		}

		return fmt.Errorf("could not start manager: %w: %s", err, string(out))
	}

	return nil
}

func (mgr *Manager) Logs() (*tail.Tail, error) {
	return tail.TailFile(
		fmt.Sprintf("/var/log/ceph/ceph-mgr.%s.log", mgr.id),
		tail.Config{Follow: true, ReOpen: true},
	)
}
