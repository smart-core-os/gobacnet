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
	"testing"
	"time"
)

func TestTSM(t *testing.T) {
	const size = 3
	tsm := New(size)
	ctx := context.Background()
	var err error
	for i := 0; i < size-1; i++ {
		_, err = tsm.ID(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}

	id, err := tsm.ID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// The buffer should be full at this point.
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	_, err = tsm.ID(ctx)
	if err == nil {
		t.Fatal("Buffer was full but an id was given ")
	}

	// Free an ID
	err = tsm.Put(id)
	if err != nil {
		t.Fatal(err)
	}

	// Now we should be able to get a new id since we free id
	_, err = tsm.ID(context.Background())
	if err != nil {
		t.Fatal(err)
	}

}

func TestTSM_ID(t *testing.T) {
	t.Run("reuses ids that are put", func(t *testing.T) {
		tsm := New(1)
		for i := 0; i < 500; i++ {
			id, err := tsm.ID(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if id == 0 {
				t.Fatal("ID was 0")
			}

			err = tsm.Put(id)
			if err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("doesn't return the same id twice", func(t *testing.T) {
		size := uint8(MaxTransaction)
		tsm := New(size)
		ids := make([]uint8, 0, size)
		for i := uint8(0); i < size; i++ {
			ctx, cleanup := context.WithTimeout(context.Background(), time.Millisecond)
			id, err := tsm.ID(ctx)
			cleanup()
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		var seen [256]bool
		for _, id := range ids {
			if seen[id] {
				t.Fatalf("id %d was returned twice", id)
			}
			seen[id] = true
		}
	})

	t.Run("don't use ids that are held", func(t *testing.T) {
		tsm := New(10)
		id, err := tsm.ID(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		for i := 0; i < MaxTransaction*2; i++ {
			id2, err := tsm.ID(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if id2 == id {
				t.Fatalf("id %d was returned twice", id)
			}
			err = tsm.Put(id2)
			if err != nil {
				t.Fatal(err)
			}
		}
	})

}

func TestConcurrency(t *testing.T) {
	const size = 10
	tsm := New(size)
	putID := make(chan uint8, size)
	sendID := make(chan uint8, size)
	recvID := make(chan uint8, size)
	done := make(chan struct{})

	// puts ids, simulating send timeouts
	errs := make(chan error, 1)
	go func() {
		defer close(putID)
		defer close(sendID)
		defer close(recvID)
		for i := 0; i < 10000; i++ {
			id, err := tsm.ID(context.Background())
			if err != nil {
				errs <- err
				return
			}
			sendID <- id
			recvID <- id
			putID <- id
		}
	}()
	go func() {
		for id := range putID {
			if err := tsm.Put(id); err != nil {
				errs <- err
				return
			}
		}
		close(done)
	}()

	for {
		select {
		case id, ok := <-sendID:
			if !ok {
				return
			}
			go tsm.Send(id, "Hello")
		case id, ok := <-recvID:
			if !ok {
				return
			}
			go tsm.Receive(context.Background(), id)
		case <-done:
		case err := <-errs:
			t.Fatal(err)
		}
	}
}

func TestDataTransaction(t *testing.T) {
	const size = 2
	tsm := New(size)
	ids := make([]uint8, size)
	var err error

	for i := 0; i < size; i++ {
		ids[i], err = tsm.ID(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}

	go func() {
		err := tsm.Send(ids[0], "Hello First ID")
		if err != nil {
			t.Error(err)
		}
	}()

	go func() {
		err := tsm.Send(ids[1], "Hello Second ID")
		if err != nil {
			t.Error(err)
		}
	}()

	done0 := make(chan struct{})
	go func() {
		defer close(done0)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		b, err := tsm.Receive(ctx, ids[0])
		if err != nil {
			t.Error(err)
		}
		s, ok := b.(string)
		if !ok {
			t.Errorf("type was not preseved")
			return
		}
		t.Log(s)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	b, err := tsm.Receive(ctx, ids[1])
	if err != nil {
		t.Error(err)
	}

	s, ok := b.(string)
	if !ok {
		t.Errorf("type was not preseved")
		return
	}
	t.Log(s)
	<-done0
}

func TestTSM_SendFrom(t *testing.T) {
	tsm := New(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id, err := tsm.IDFor(ctx, "10.0.0.1:47808")
	if err != nil {
		t.Fatal(err)
	}
	if err := tsm.SendFrom(id, "10.0.0.2:47808", "stray"); err == nil {
		t.Fatal("delivered a reply from the wrong peer")
	}
	go func() {
		if err := tsm.SendFrom(id, "10.0.0.1:47808", "reply"); err != nil {
			t.Error(err)
		}
	}()
	got, err := tsm.Receive(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got != "reply" {
		t.Fatalf("got %v, want the matching peer's reply", got)
	}

	// the refused send must not hold on to the slot
	if err := tsm.Put(id); err != nil {
		t.Fatal(err)
	}
	id, err = tsm.IDFor(ctx, "")
	if err != nil {
		t.Fatalf("slot not released: %v", err)
	}
	// an empty peer, as for a broadcast, accepts a reply from anywhere
	go func() {
		if err := tsm.SendFrom(id, "10.0.0.3:47808", "any"); err != nil {
			t.Error(err)
		}
	}()
	if got, err := tsm.Receive(ctx, id); err != nil || got != "any" {
		t.Fatalf("got %v, %v", got, err)
	}
}
