// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package service

import (
	"context"
)

type Service interface {
	Start(ctx context.Context) error
	Stop() error
	Name() string
}
