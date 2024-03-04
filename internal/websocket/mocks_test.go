// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package websocket

import (
	mock "github.com/stretchr/testify/mock"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/xmidt-agent/internal/websocket/event"
)

// MockService is an autogenerated mock type for the Service type
type MockListeners struct {
	mock.Mock
}

func (m *MockListeners) OnConnect(e event.Connect) {
	m.Called(e)
}

func (m *MockListeners) OnDisconnect(e event.Disconnect) {
	m.Called(e)
}

func (m *MockListeners) OnHeartbeat(e event.Heartbeat) {
	m.Called(e)
}

func (m *MockListeners) OnMessage(w wrp.Message) {
	m.Called(w)
}

func (m *MockListeners) OnDecodeError(e error) {
	m.Called(e)
}