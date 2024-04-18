// SPDX-License-Identifier: MPL-2.0
/*
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/dpeckett/picoceph/internal/ceph"
	"github.com/nxadm/tail"
)

type Dashboard struct{}

func New() ceph.Component {
	return &Dashboard{}
}

func (d *Dashboard) Name() string {
	return "dashboard"
}

func (d *Dashboard) Configure(ctx context.Context) error {
	// Don't block forever if ceph does not come up.
	cephCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Wait for the mgr module to be ready.
	for {
		select {
		case <-cephCtx.Done():
			return fmt.Errorf("timed out waiting for dashboard module to load")
		default:

			cmd := exec.CommandContext(cephCtx, "ceph", "mgr", "module", "ls", "--format=json")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("could not list mgr modules: %w: %s", err, string(out))
			}

			// Parse the JSON output.
			var moduleList struct {
				AlwaysOnModules []string `json:"always_on_modules"`
				EnabledModules  []string `json:"enabled_modules"`
				DisabledModules []struct {
					Name string `json:"name"`
				} `json:"disabled_modules"`
			}

			if err := json.Unmarshal(out, &moduleList); err != nil {
				return fmt.Errorf("could not parse mgr modules: %w: %s", err, string(out))
			}

			// Check if the dashboard module is loaded.
			var modules []string
			modules = append(modules, moduleList.AlwaysOnModules...)
			modules = append(modules, moduleList.EnabledModules...)
			for _, mod := range moduleList.DisabledModules {
				modules = append(modules, mod.Name)
			}

			for _, module := range modules {
				if module == "dashboard" {
					return nil
				}
			}

			// If the module is not loaded, wait a bit and try again.
			time.Sleep(time.Second)
		}
	}
}

func (d *Dashboard) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ceph", "mgr", "module", "enable", "dashboard")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not enable dashboard: %w: %s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "ceph", "config", "set", "mgr", "mgr/dashboard/ssl", "false")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not disable SSL for dashboard: %w: %s", err, string(out))
	}

	return nil
}

func (d *Dashboard) Logs() (*tail.Tail, error) {
	// Dashboard logs are logged by the manager.
	return tail.TailFile(
		"/dev/null",
		tail.Config{Follow: true, ReOpen: true},
	)
}
