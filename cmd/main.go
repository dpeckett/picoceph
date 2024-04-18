// SPDX-License-Identifier: MPL-2.0
/*
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dpeckett/picoceph/internal/ceph"
	"github.com/dpeckett/picoceph/internal/ceph/dashboard"
	"github.com/dpeckett/picoceph/internal/ceph/manager"
	"github.com/dpeckett/picoceph/internal/ceph/monitor"
	"github.com/dpeckett/picoceph/internal/ceph/osd"
	"github.com/dpeckett/picoceph/internal/ceph/radosgw"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))

	logger.Info("Creating ceph directories")

	cephUserUid, cephGroupGid, err := ceph.User()
	if err != nil {
		logger.Error("Could not get ceph user", "error", err)
		os.Exit(1)
	}

	for _, dir := range []string{"/etc/ceph", "/var/lib/ceph", "/var/log/ceph"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logger.Error("Could not create directory", "error", err)
			os.Exit(1)
		}

		if err := os.Chown(dir, cephUserUid, cephGroupGid); err != nil {
			logger.Error("Could not change owner", "error", err)
			os.Exit(1)
		}
	}

	logger.Info("Writing ceph.conf")

	fsid := uuid.New().String()

	if err := ceph.WriteConfig(fsid); err != nil {
		logger.Error("Could not write ceph.conf", "error", err)
		os.Exit(1)
	}

	g, ctx := errgroup.WithContext(ctx)

	components := []ceph.Component{
		monitor.New("a", fsid),
		manager.New("a"),
		osd.New("0"),
		radosgw.New(),
		dashboard.New(),
	}

	for _, cmp := range components {
		cmp := cmp

		g.Go(func() error {
			logger.Info("Configuring", "component", cmp.Name())

			if err := cmp.Configure(ctx); err != nil {
				return fmt.Errorf("could not configure component: %w", err)
			}

			// Start echoing logs from the component.
			go func() {
				t, err := cmp.Logs()
				if err != nil {
					logger.Error("Could not tail logs", "error", err)
					return
				}
				defer t.Cleanup()

				for line := range t.Lines {
					logger.Info(line.Text, "component", cmp.Name())
				}
			}()

			logger.Info("Starting", "component", cmp.Name())

			if err := cmp.Start(ctx); err != nil {
				return fmt.Errorf("could not start component: %w", err)
			}

			return nil
		})
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh

		logger.Info("Shutting down")

		cancel()
	}()

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("Could not run picoceph", "error", err)

		os.Exit(1)
	}
}
