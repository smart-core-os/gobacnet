/*Copyright (C) 2017 Alex Beltran

This program is free software; you can redistribute it and/or
modify it under the terms of the GNU General Public License
as published by the Free Software Foundation; either version 2
of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program; if not, write to:
The Free Software Foundation, Inc.
59 Temple Place - Suite 330
Boston, MA  02111-1307, USA.

As a special exception, if other files instantiate templates or
use macros or inline functions from this file, or you compile
this file and link it with other works to produce a work based
on this file, this file does not by itself cause the resulting
work to be covered by the GNU General Public License. However
the source code for this file must still be made available in
accordance with section (3) of the GNU General Public License.

This exception does not invalidate any other reasons why a work
based on this file might be covered by the GNU General Public
License.
*/

package tsm

import (
	"context"
	"fmt"
	"sync"
)

// MaxTransaction is the default max number of transactions that can occur
// concurrently. That's one for every uint8 number except 0.
const MaxTransaction = 255
const invalidID = 0

type state struct {
	peer      string // where the request went, replies from anywhere else are refused. Empty matches any peer
	data      chan interface{}
	reclaimed chan struct{} // closed when Put is called for the state id
	sends     int32         // must hold TSM.mutex when accessing this variable
}

// TSM is the transaction state manager. It handles passing data to other
// processes and keeping track of what transactions are currently processed
type TSM struct {
	mutex  sync.Mutex
	states map[uint8]*state
	pool   sync.Pool
	free   struct {
		id    chan uint8    // queue of free ids, in whatever order they were last released
		space chan struct{} // free concurrent transaction slots, allows concurrent transactions to be less than MaxTransaction
	}
}

// New creates a new transaction manager that can handle at most size concurrent transactions.
func New(size uint8) *TSM {
	if size > MaxTransaction {
		panic(fmt.Sprintf("size %d is greater than MaxTransaction %d", size, MaxTransaction))
	}
	t := &TSM{
		states: make(map[uint8]*state),
		pool: sync.Pool{
			// Operation doesn't include a new channel. We want that done when a get is
			// done since we close all channels when putting into the pool.
			New: func() interface{} {
				s := new(state)
				return s
			},
		},
	}

	// Generate free ids.
	t.free.id = make(chan uint8, MaxTransaction)
	for i := invalidID + 1; i <= MaxTransaction; i++ {
		t.free.id <- uint8(i)
	}

	// Generate free space
	t.free.space = make(chan struct{}, size)
	for i := uint8(0); i < size; i++ {
		t.free.space <- struct{}{}
	}

	return t
}

// Send data to invoked id, from any peer.
func (t *TSM) Send(id uint8, b interface{}) error {
	return t.SendFrom(id, "", b)
}

// SendFrom sends data to invoked id, if the transaction is with peer.
// An empty peer, here or in IDFor, matches any peer.
func (t *TSM) SendFrom(id uint8, peer string, b interface{}) error {
	t.mutex.Lock()
	s, ok := t.states[id]
	if !ok {
		t.mutex.Unlock()
		return fmt.Errorf("id %d is not receiving", id)
	}
	if peer != "" && s.peer != "" && peer != s.peer {
		t.mutex.Unlock()
		return fmt.Errorf("id %d is waiting on %s, not %s", id, s.peer, peer)
	}
	s.sends++
	data := s.data
	t.mutex.Unlock()

	select {
	case data <- b:
		// There's a chance that between sending and entering the below lock someone Put the id.
		// In that case when Put sees s.sends it will be >0 so it won't close it, so we have to check.
		t.mutex.Lock()
		s.sends--
		if s.sends == 0 {
			select {
			case <-s.reclaimed:
				t.reclaim(id, s)
			default:
			}
		}
		t.mutex.Unlock()
	case <-s.reclaimed:
		t.mutex.Lock()
		s.sends--
		t.reclaim(id, s)
		t.mutex.Unlock()
		return fmt.Errorf("id %d is not receiving", id)
	}
	return nil
}

// Receive attempts to receive a byte array from the invoked id
func (t *TSM) Receive(ctx context.Context, id uint8) (interface{}, error) {
	t.mutex.Lock()
	s, ok := t.states[id]
	if !ok {
		t.mutex.Unlock()
		return nil, fmt.Errorf("id %d is not sending", id)
	}
	data := s.data
	t.mutex.Unlock()

	// Wait for data
	select {
	case b, ok := <-data:
		if !ok {
			return nil, fmt.Errorf("id %d is not sending", id)
		}
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ID returns the invoke id that was used to save the state of this connection.
func (t *TSM) ID(ctx context.Context) (uint8, error) {
	return t.IDFor(ctx, "")
}

// IDFor is like ID, for a transaction with peer: SendFrom only delivers replies from peer.
func (t *TSM) IDFor(ctx context.Context, peer string) (uint8, error) {
	var id uint8
	select {
	case <-t.free.space:
		// got a free spot, lets try and get a free id
		select {
		case id = <-t.free.id:
		case <-ctx.Done():
			t.free.space <- struct{}{}
			return 0, fmt.Errorf("unable to get a free id: %w", ctx.Err())
		}
	case <-ctx.Done():
		return 0, fmt.Errorf("no free space: %w", ctx.Err())
	}

	// skip error checking, since we control new generation and what is put in the pool.
	s := t.pool.Get().(*state)
	t.mutex.Lock()
	s.peer = peer
	s.data = make(chan interface{})
	s.reclaimed = make(chan struct{})
	if s.sends > 0 {
		panic("s.sends should be 0")
	}
	t.states[id] = s
	t.mutex.Unlock()
	return id, nil
}

// Put allows the id to be reused in the transaction manager.
// Put only returns an error if id is not known.
func (t *TSM) Put(id uint8) error {
	t.mutex.Lock()
	s, ok := t.states[id]
	if !ok {
		t.mutex.Unlock()
		return fmt.Errorf("id %d does not exist in the transactions", id)
	}
	delete(t.states, id)
	close(s.reclaimed) // notify
	t.reclaim(id, s)
	t.mutex.Unlock()
	return nil
}

func (t *TSM) reclaim(id uint8, s *state) {
	if s.sends == 0 {
		close(s.data)
		t.pool.Put(s)
		t.free.id <- id
		t.free.space <- struct{}{}
	}
}
