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
)

type Manager struct{}

func New() ceph.Component {
	return &Manager{}
}

func (m *Manager) Name() string {
	return "manager"
}

func (m *Manager) Configure(ctx context.Context) error {
	if err := os.MkdirAll("/var/lib/ceph/mgr/ceph-a", 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	mgrKeyring, err := os.Create("/var/lib/ceph/mgr/ceph-a/keyring")
	if err != nil {
		return fmt.Errorf("could not create keyring: %w", err)
	}
	defer mgrKeyring.Close()

	// Don't block forever if ceph does not come up.
	cephCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(cephCtx, "ceph", "auth", "get-or-create", "mgr.a", "mon", "allow profile mgr", "osd", "allow *", "mds", "allow *")
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

	if err := util.ChownRecursive("/var/lib/ceph/mgr/ceph-a", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ceph-mgr", "-f", "-i", "a")
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			return nil
		}

		return fmt.Errorf("could not start manager: %w: %s", err, string(out))
	}

	return nil
}
