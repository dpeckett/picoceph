// SPDX-License-Identifier: MPL-2.0
/*
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package radosgw

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dpeckett/picoceph/internal/ceph"
	"github.com/dpeckett/picoceph/internal/util"
	"github.com/nxadm/tail"
)

type RADOSGW struct{}

func New() ceph.Component {
	return &RADOSGW{}
}

func (rgw *RADOSGW) Name() string {
	return "rgw.gateway"
}

func (rgw *RADOSGW) Configure(ctx context.Context) error {
	if err := os.MkdirAll("/var/lib/ceph/radosgw/ceph-radosgw.gateway", 0o755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	radosgwKeyring, err := os.Create("/var/lib/ceph/radosgw/ceph-radosgw.gateway/keyring")
	if err != nil {
		return fmt.Errorf("could not create keyring: %w", err)
	}
	defer radosgwKeyring.Close()

	// Don't block forever if ceph does not come up.
	cephCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(cephCtx, "ceph", "auth", "get-or-create", "client.radosgw.gateway", "osd", "allow rwx", "mon", "allow rw")
	cmd.Stdout = radosgwKeyring

	var out strings.Builder
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not create keyring: %w: %s", err, out.String())
	}

	cephUserUid, cephGroupGid, err := ceph.User()
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	if err := util.ChownRecursive("/var/lib/ceph/radosgw/ceph-radosgw.gateway", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}

func (rgw *RADOSGW) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "radosgw", "-f", "-n", "client.radosgw.gateway")
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			return nil
		}

		return fmt.Errorf("could not start RADOS Gateway: %w: %s", err, out)
	}

	return nil
}

func (rgw *RADOSGW) Logs() (*tail.Tail, error) {
	return tail.TailFile(
		"/var/log/ceph/ceph-client.radosgw.gateway.log",
		tail.Config{Follow: true, ReOpen: true},
	)
}
