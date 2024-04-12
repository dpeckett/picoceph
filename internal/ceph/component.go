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
	"context"

	"github.com/nxadm/tail"
)

// Component is a Ceph component eg. monitor, dashboard, etc.
type Component interface {
	// Name returns the name of the component.
	Name() string
	// Configure configures the component (eg. writes config files, creates directories, etc.)
	Configure(ctx context.Context) error
	// Start starts the component.
	Start(ctx context.Context) error
	// Logs returns the logs of the component.
	Logs() (*tail.Tail, error)
}
