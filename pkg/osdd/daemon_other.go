//go:build !linux

package osdd

import "context"

func Start(context.Context) func() { return func() {} }
