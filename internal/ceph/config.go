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
	"os"
	"text/template"

	_ "embed"
)

//go:embed assets/ceph.conf.tmpl
var cephConfTmpl string

// WriteConfig writes the ceph.conf file.
func WriteConfig(fsid string) error {
	cephConf, err := os.Create("/etc/ceph/ceph.conf")
	if err != nil {
		return fmt.Errorf("could not create ceph.conf: %w", err)
	}
	defer cephConf.Close()

	tmpl, err := template.New("ceph.conf").Parse(cephConfTmpl)
	if err != nil {
		return fmt.Errorf("could not parse ceph.conf template: %w", err)
	}

	if err := tmpl.Execute(cephConf, struct {
		FSID string
	}{
		FSID: fsid,
	}); err != nil {
		return fmt.Errorf("could not execute ceph.conf template: %w", err)
	}

	cephUserUid, cephGroupGid, err := User()
	if err != nil {
		return fmt.Errorf("could not get ceph user: %w", err)
	}

	if err := os.Chown("/etc/ceph/ceph.conf", cephUserUid, cephGroupGid); err != nil {
		return fmt.Errorf("could not change owner: %w", err)
	}

	return nil
}
