// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

//물리적인 통신 수단(채널, TCP, UDP 등)을 추상화

package network

// Sender 인터페이스: 어디로든 보낼 수 있다는 약속일세.
type Sender interface {
	Send(msg interface{}) error
}

// LocalSender: 고루틴 간 채널 통신용 (시뮬레이션용)
type LocalSender struct {
	TargetChan chan interface{}
}

func (s *LocalSender) Send(msg interface{}) error {
	s.TargetChan <- msg
	return nil
}
