/*
 * Copyright (C) 2020 The "MysteriumNetwork/node" Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package traversal

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/mysteriumnetwork/node/core/port"
	"github.com/stretchr/testify/assert"
)

func TestPinger_Multiple_Stop(t *testing.T) {
	pinger := newPinger(&PingConfig{
		Interval: 1 * time.Millisecond,
		Timeout:  10 * time.Millisecond,
	})

	// Make sure multiple stops doesn't crash.
	pinger.Stop()
	pinger.Stop()
	pinger.Stop()
}

func TestPinger_Provider_Consumer_Ping_Flow(t *testing.T) {
	providerProxyPort := 51199
	providerPort := 51200
	consumerPort := 51201

	pingConfig := &PingConfig{
		Interval: 10 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
	}

	pinger := newPinger(pingConfig)
	defer pinger.Stop()

	// Create provider's UDP proxy listener to which pinger should hand off connection.
	// In real world this proxy represents started VPN service (WireGuard or OpenVPN).
	ch := make(chan string)
	go func() {
		addr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%d", providerProxyPort))
		conn, err := net.ListenUDP("udp4", addr)
		assert.NoError(t, err)

		proxyBuf := make([]byte, 1024)

		for {
			n, err := conn.Read(proxyBuf)
			assert.NoError(t, err)
			ch <- string(proxyBuf[:n])
		}
	}()

	// Start pinging consumer.
	go func() {
		pinger.BindServicePort("wg1", providerProxyPort)
		pinger.PingConsumer("127.0.0.1", []int{providerPort}, []int{consumerPort}, "wg1")
	}()

	// Wait some time to simulate real network delay conditions.
	time.Sleep(5 * pingConfig.Interval)

	_, _, err := pinger.PingProvider("127.0.0.1", []int{consumerPort}, []int{providerPort}, consumerPort+1)
	assert.NoError(t, err)

	laddr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%d", consumerPort))
	raddr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%d", providerPort))

	conn, err := net.DialUDP("udp4", laddr, raddr)
	assert.NoError(t, err)

	defer conn.Close()

	assert.Eventually(t, func() bool {
		conn.Write([]byte("Test message"))
		select {
		case msg := <-ch:
			if msg == "Test message" {
				return true
			}
		default:
		}
		return false
	}, time.Second, 10*time.Millisecond)
}

func TestPinger_PingPeer_N_Connections(t *testing.T) {
	pingConfig := &PingConfig{
		Interval: 10 * time.Millisecond,
		Timeout:  1000 * time.Millisecond,
	}

	provider := newPinger(pingConfig)
	consumer := newPinger(pingConfig)

	var pPorts, cPorts []int
	ports, err := port.NewPool().AcquireMultiple(10)
	assert.NoError(t, err)

	for i := 0; i < 5; i++ {
		pPorts = append(pPorts, ports[i].Num())
		cPorts = append(cPorts, ports[5+i].Num())
	}

	go consumer.PingPeer("127.0.0.1", cPorts, pPorts, 2, 3)
	conns, err := provider.PingPeer("127.0.0.1", pPorts, cPorts, 2, 3)
	assert.NoError(t, err)

	assert.Len(t, conns, 3)
}

func TestPinger_PingPeer_Not_Enough_Connections(t *testing.T) {
	pingConfig := &PingConfig{
		Interval: 10 * time.Millisecond,
		Timeout:  1000 * time.Millisecond,
	}

	provider := newPinger(pingConfig)
	consumer := newPinger(pingConfig)

	var pPorts, cPorts []int
	ports, err := port.NewPool().AcquireMultiple(10)
	assert.NoError(t, err)

	for i := 0; i < 5; i++ {
		pPorts = append(pPorts, ports[i].Num())
		cPorts = append(cPorts, ports[5+i].Num())
	}

	go consumer.PingPeer("127.0.0.1", cPorts, pPorts, 2, 30)
	conns, err := provider.PingPeer("127.0.0.1", pPorts, cPorts, 2, 30)
	assert.EqualError(t, err, "not enough connections")

	assert.Len(t, conns, 5)
}

func TestPinger_PingProvider_Timeout(t *testing.T) {
	pinger := newPinger(&PingConfig{
		Interval: 1 * time.Millisecond,
		Timeout:  5 * time.Millisecond,
	})

	providerPort := 51205
	consumerPort := 51206

	go func() {
		addr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%d", providerPort))
		conn, err := net.ListenUDP("udp4", addr)
		assert.NoError(t, err)
		defer conn.Close()

		select {}
	}()

	_, _, err := pinger.PingProvider("127.0.0.1", []int{consumerPort}, []int{providerPort}, 0)

	assert.Error(t, errNATPunchAttemptTimedOut, err)
}

func newPinger(config *PingConfig) NATPinger {
	return NewPinger(config, &mockPublisher{})
}

type mockPublisher struct {
}

func (p mockPublisher) Publish(topic string, data interface{}) {
}
