// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
)

// --- messages ---------------------------------------------------------------

type (
	Telemetry struct {
		Temp     float64
		Humidity float64
	}
	GetStatus struct{}
	Status    struct {
		LastTemp     float64
		LastHumidity float64
		ReadingsSeen int
		LastSeenAt   time.Time
		Activated    time.Time
	}
)

// --- in-memory state store --------------------------------------------------
//
// A grain keeps state in memory while active; on deactivation it persists a
// snapshot here so the next activation can restore. In a real deployment this
// would be a database or a goakt persistence extension.

type snapshot struct {
	LastTemp     float64
	LastHumidity float64
	ReadingsSeen int
	LastSeenAt   time.Time
}

type store struct {
	mu   sync.Mutex
	data map[string]snapshot
}

func newStore() *store { return &store{data: make(map[string]snapshot)} }

func (s *store) load(id string) (snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.data[id]
	return snap, ok
}

func (s *store) save(id string, snap snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = snap
}

// --- the grain --------------------------------------------------------------

// DeviceTwin is a virtual actor that represents one physical device. The
// runtime activates it on demand when the first message for that device id
// arrives, and deactivates it after a configurable idle period.
type DeviceTwin struct {
	store     *store
	id        string
	state     snapshot
	activated time.Time
}

var _ actor.Grain = (*DeviceTwin)(nil)

func (x *DeviceTwin) OnActivate(_ context.Context, props *actor.GrainProps) error {
	x.id = props.Identity().Name()
	x.activated = time.Now()
	if snap, ok := x.store.load(x.id); ok {
		x.state = snap
		props.ActorSystem().Logger().Infof("[%s] activated, restored %d readings", x.id, snap.ReadingsSeen)
	} else {
		props.ActorSystem().Logger().Infof("[%s] activated, fresh state", x.id)
	}
	return nil
}

func (x *DeviceTwin) OnDeactivate(_ context.Context, props *actor.GrainProps) error {
	props.ActorSystem().Logger().Infof("[%s] deactivating, persisting %d readings", x.id, x.state.ReadingsSeen)
	x.store.save(x.id, x.state)
	return nil
}

func (x *DeviceTwin) OnReceive(ctx *actor.GrainContext) {
	switch msg := ctx.Message().(type) {
	case *Telemetry:
		x.state.LastTemp = msg.Temp
		x.state.LastHumidity = msg.Humidity
		x.state.ReadingsSeen++
		x.state.LastSeenAt = time.Now()
		ctx.NoErr()
	case *GetStatus:
		ctx.Response(&Status{
			LastTemp:     x.state.LastTemp,
			LastHumidity: x.state.LastHumidity,
			ReadingsSeen: x.state.ReadingsSeen,
			LastSeenAt:   x.state.LastSeenAt,
			Activated:    x.activated,
		})
	default:
		ctx.Unhandled()
	}
}

// --- demo -------------------------------------------------------------------

func main() {
	ctx := context.Background()
	logger := log.DiscardLogger

	system, err := actor.NewActorSystem("IoTTwin", actor.WithLogger(logger))
	if err != nil {
		logger.Fatal(err)
	}

	if err := system.Start(ctx); err != nil {
		logger.Fatal(err)
	}

	defer func() { _ = system.Stop(ctx) }()

	st := newStore()

	// Short deactivation window so the demo can show passivation in action.
	const idleWindow = 2 * time.Second
	identity := func(deviceID string) *actor.GrainIdentity {
		id, err := system.GrainIdentity(ctx, deviceID,
			func(_ context.Context) (actor.Grain, error) {
				return &DeviceTwin{store: st}, nil
			},
			actor.WithGrainDeactivateAfter(idleWindow),
		)
		if err != nil {
			logger.Fatal(err)
		}
		return id
	}

	// 1) First telemetry for three devices — each grain activates on demand.
	logger.Info("--- ingesting telemetry for 3 devices ---")
	for _, dev := range []string{"sensor-A", "sensor-B", "sensor-C"} {
		_ = system.TellGrain(ctx, identity(dev),
			&Telemetry{Temp: 21.4, Humidity: 55})
		_ = system.TellGrain(ctx, identity(dev),
			&Telemetry{Temp: 21.6, Humidity: 56})
	}

	// 2) Query one of them while still active. AskGrain blocks until the
	//    grain replies, and the mailbox is FIFO, so the two Tells above
	//    are guaranteed to be processed before this Ask.
	resp, err := system.AskGrain(ctx, identity("sensor-A"), &GetStatus{}, time.Second)
	if err != nil {
		logger.Fatal(err)
	}
	s := resp.(*Status)
	fmt.Printf("sensor-A status: temp=%.1f humidity=%.1f readings=%d\n",
		s.LastTemp, s.LastHumidity, s.ReadingsSeen)

	// 3) Sit idle past the deactivation window — grains passivate.
	logger.Infof("--- idling %s to trigger passivation ---", idleWindow+500*time.Millisecond)
	time.Sleep(idleWindow + 500*time.Millisecond)

	// 4) Send telemetry again — sensor-A reactivates and should restore
	//    the previous reading count from the store.
	logger.Info("--- new telemetry on sensor-A: should reactivate ---")
	_ = system.TellGrain(ctx, identity("sensor-A"),
		&Telemetry{Temp: 22.0, Humidity: 57})

	resp, err = system.AskGrain(ctx, identity("sensor-A"), &GetStatus{}, time.Second)
	if err != nil {
		logger.Fatal(err)
	}
	s = resp.(*Status)
	fmt.Printf("sensor-A after reactivation: temp=%.1f humidity=%.1f readings=%d\n",
		s.LastTemp, s.LastHumidity, s.ReadingsSeen)
}
