// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

// transport.go abstracts physical communication layers (Channels, TCP, UDP, etc.)

package network

/// Sender defines a contract for transmitting data to a destination.
/// This abstraction allows the system to remain agnostic of the underlying network protocol.
type Sender interface {
	Send(msg interface{}) error
}

/// LocalSender implements the Sender interface using Go channels.
/// Primarily used for local simulations and inter-goroutine communication.
type LocalSender struct {
	TargetChan chan interface{}
}

/// Send dispatches a message into the target channel.
func (s *LocalSender) Send(msg interface{}) error {
	s.TargetChan <- msg
	return nil
}
