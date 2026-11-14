/*
 * @Author: lwnmengjing
 * @Date: 2021/6/7 5:54 下午
 * @Last Modified by: lwnmengjing
 * @Last Modified time: 2021/6/7 5:54 下午
 */

package server

import "time"

// Option configures the service manager before it begins running.
type Option func(*options)

type options struct {
	gracefulShutdownTimeout time.Duration
}

func setDefaultOptions() options {
	return options{
		gracefulShutdownTimeout: 5 * time.Second,
	}
}

// WithShutdownTimeout sets the total budget shared by service shutdowns.
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.gracefulShutdownTimeout = d
		}
	}
}
