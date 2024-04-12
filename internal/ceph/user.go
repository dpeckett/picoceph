// SPDX-License-Identifier: MPL-2.0
/*
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package ceph

import (
	"fmt"
	"os/user"
	"strconv"
)

// User returns the uid and gid of the ceph user and group.
func User() (int, int, error) {
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
