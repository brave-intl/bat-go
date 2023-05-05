// Package repository provides access to data available in SQL-based data store.
package repository

import "context"

type Orders struct{}

func (r *Orders) Get(ctx context.Context)
