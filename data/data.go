/* SPDX-License-Identifier: MPL-2.0
 *
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package data

import (
	_ "embed"
	"text/template"
)

//go:embed ceph.conf.tmpl
var CephConfTmpl string

// GetCephConfTmpl returns the ceph.conf template.
func GetCephConfTmpl() *template.Template {
	tmpl, err := template.New("ceph.conf").Parse(CephConfTmpl)
	if err != nil {
		panic(err)
	}

	return tmpl
}
