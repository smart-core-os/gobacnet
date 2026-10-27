package gobacnet

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/smart-core-os/gobacnet/encoding"
	"github.com/smart-core-os/gobacnet/enum/errorclass"
	"github.com/smart-core-os/gobacnet/enum/errorcode"
	"github.com/smart-core-os/gobacnet/enum/pdutype"
	"github.com/smart-core-os/gobacnet/property"
	"github.com/smart-core-os/gobacnet/types"
	"github.com/smart-core-os/gobacnet/types/objecttype"
)

// fakeDevice answers the client's confirmed requests with whatever its handler returns,
// so tests can send the replies, or lack of them, a real device might.
type fakeDevice struct {
	t    *testing.T
	conn *net.UDPConn
	addr *net.UDPAddr

	mu       sync.Mutex
	requests []types.APDU
}

// fakeHandler returns the APDU to reply with, or nil to send no reply.
type fakeHandler func(from *net.UDPAddr, req types.APDU) []byte

func startFakeDevice(t *testing.T, handle fakeHandler) *fakeDevice {
	t.Helper()
	conn := listenLoopback(t)
	d := &fakeDevice{t: t, conn: conn, addr: conn.LocalAddr().(*net.UDPAddr)}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			dec := encoding.NewDecoder(buf[4:n])
			var npdu types.NPDU
			var apdu types.APDU
			if dec.NPDU(&npdu) != nil || dec.APDU(&apdu) != nil || apdu.DataType != pdutype.ConfirmedServiceRequest {
				continue
			}
			d.mu.Lock()
			d.requests = append(d.requests, apdu)
			d.mu.Unlock()
			if reply := handle(from, apdu); reply != nil {
				sendAPDU(t, conn, from, reply)
			}
		}
	}()
	return d
}

func (d *fakeDevice) device() types.Device {
	return types.Device{ID: types.ObjectID{Type: objecttype.Device, Instance: 1}, Addr: types.UDPToAddress(d.addr), MaxApdu: encoding.MaxAPDU}
}

// requestCount returns how many requests for service the device has seen, waiting up to wait for there to be want.
func (d *fakeDevice) requestCount(service types.ServiceConfirmed, want int, wait time.Duration) int {
	deadline := time.Now().Add(wait)
	for {
		d.mu.Lock()
		n := 0
		for _, r := range d.requests {
			if r.Service == service {
				n++
			}
		}
		d.mu.Unlock()
		if n >= want || time.Now().After(deadline) {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func listenLoopback(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// sendAPDU sends apdu from conn to the client at to, wrapped as a BACnet/IP reply.
func sendAPDU(t *testing.T, conn *net.UDPConn, to *net.UDPAddr, apdu []byte) {
	e := encoding.NewEncoder()
	e.NPDU(types.NPDU{Version: types.ProtocolVersion})
	payload := append(e.Bytes(), apdu...)
	e = encoding.NewEncoder()
	_ = e.BVLC(types.BVLC{Type: types.BVLCTypeBacnetIP, Function: types.BacFuncUnicast, Length: uint16(4 + len(payload)), Data: payload})
	if _, err := conn.WriteToUDP(e.Bytes(), to); err != nil {
		t.Error(err)
	}
}

func encodeAPDU(t *testing.T, f func(e *encoding.Encoder) error) []byte {
	t.Helper()
	e := encoding.NewEncoder()
	if err := f(e); err != nil {
		t.Fatal(err)
	}
	return e.Bytes()
}

func abortAPDU(t *testing.T, req types.APDU, reason types.AbortReason) []byte {
	return encodeAPDU(t, func(e *encoding.Encoder) error { return e.AbortPDU(req.InvokeId, true, reason) })
}

// readPropertyAck answers a ReadProperty request with the value in values, or an unknown-property error.
func readPropertyAck(t *testing.T, req types.APDU, values map[types.ObjectID]map[property.ID]interface{}) []byte {
	var rp types.ReadPropertyData
	if err := encoding.NewDecoder(req.RawData).ReadProperty(&rp); err != nil || len(rp.Object.Properties) != 1 {
		t.Errorf("decode ReadProperty request: %v", err)
		return nil
	}
	v, ok := values[rp.Object.ID][rp.Object.Properties[0].ID]
	if !ok {
		bacErr := types.Error{Class: errorclass.Property, Code: errorcode.UnknownProperty}
		return encodeAPDU(t, func(e *encoding.Encoder) error {
			return e.ErrorPDU(req.InvokeId, types.ServiceConfirmedReadProperty, bacErr)
		})
	}
	rp.Object.Properties[0].Data = v
	return encodeAPDU(t, func(e *encoding.Encoder) error { return e.ReadPropertyAck(req.InvokeId, rp) })
}

func newTestClient(t *testing.T, opts ...Option) *Client {
	t.Helper()
	free, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		t.Fatal(err)
	}
	port := free.LocalAddr().(*net.UDPAddr).Port
	_ = free.Close()

	opts = append([]Option{WithLogLevel(logrus.WarnLevel)}, opts...)
	c, err := NewClient(ipv4InterfaceName(t), port, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// ipv4InterfaceName finds an interface the client will accept: it needs one with an IPv4 address.
func ipv4InterfaceName(t *testing.T) string {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ip, _, _ := net.ParseCIDR(a.String()); ip.To4() != nil {
				return iface.Name
			}
		}
	}
	t.Skip("no interface with an IPv4 address")
	return ""
}

func testCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func avObject(i types.ObjectInstance) types.ObjectID {
	return types.ObjectID{Type: objecttype.AnalogValue, Instance: i}
}

func readPV(ctx context.Context, c *Client, dev types.Device, obj types.ObjectID) (types.ReadPropertyData, error) {
	return c.ReadProperty(ctx, dev, types.ReadPropertyData{Object: types.Object{
		ID:         obj,
		Properties: []types.Property{{ID: property.PresentValue, ArrayIndex: ArrayAll}},
	}})
}

func wantBACnetError(t *testing.T, err error, want types.Error) {
	t.Helper()
	var got types.Error
	if !errors.As(err, &got) || got != want {
		t.Fatalf("got error %v, want %v", err, want)
	}
}

func TestClient_ReadProperty_abort(t *testing.T) {
	values := map[types.ObjectID]map[property.ID]interface{}{avObject(2): {property.PresentValue: float32(21.5)}}
	dev := startFakeDevice(t, func(_ *net.UDPAddr, req types.APDU) []byte {
		if req.Service != types.ServiceConfirmedReadProperty {
			return nil
		}
		var rp types.ReadPropertyData
		_ = encoding.NewDecoder(req.RawData).ReadProperty(&rp)
		if rp.Object.ID == avObject(1) {
			return abortAPDU(t, req, types.AbortSegmentationNotSupported)
		}
		return readPropertyAck(t, req, values)
	})
	// one transaction slot: the second read only gets one if the aborted read freed it
	c := newTestClient(t, WithMaxConcurrentTransactions(1))

	// no deadline: before the fix this blocked for good
	done := make(chan error, 1)
	go func() {
		_, err := readPV(context.Background(), c, dev.device(), avObject(1))
		done <- err
	}()
	select {
	case err := <-done:
		wantBACnetError(t, err, types.Error{Class: errorclass.Communication, Code: errorcode.AbortSegmentationNotSupported})
	case <-time.After(5 * time.Second):
		t.Fatal("ReadProperty did not return after an Abort")
	}

	res, err := readPV(testCtx(t), c, dev.device(), avObject(2))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Object.Properties[0].Data; got != float32(21.5) {
		t.Fatalf("got %v, want 21.5", got)
	}
}

func TestClient_ReadMultiProperty_reject(t *testing.T) {
	dev := startFakeDevice(t, func(_ *net.UDPAddr, req types.APDU) []byte {
		return encodeAPDU(t, func(e *encoding.Encoder) error { return e.RejectPDU(req.InvokeId, types.RejectUnrecognizedService) })
	})
	c := newTestClient(t)

	_, err := c.ReadMultiProperty(testCtx(t), dev.device(), types.ReadMultipleProperty{Objects: []types.Object{{
		ID:         avObject(1),
		Properties: []types.Property{{ID: property.PresentValue, ArrayIndex: ArrayAll}},
	}}})
	wantBACnetError(t, err, types.Error{Class: errorclass.Communication, Code: errorcode.RejectUnrecognizedService})
}

func TestClient_ReadProperties_fallback(t *testing.T) {
	values := map[types.ObjectID]map[property.ID]interface{}{
		avObject(1): {property.PresentValue: float32(1.5), property.ObjectName: "one"},
		avObject(2): {property.PresentValue: float32(2.5), property.ObjectName: "two"},
	}
	dev := startFakeDevice(t, func(_ *net.UDPAddr, req types.APDU) []byte {
		if req.Service == types.ServiceConfirmedReadPropMultiple {
			return abortAPDU(t, req, types.AbortSegmentationNotSupported)
		}
		return readPropertyAck(t, req, values)
	})
	c := newTestClient(t)

	props := []types.Property{{ID: property.PresentValue, ArrayIndex: ArrayAll}, {ID: property.ObjectName, ArrayIndex: ArrayAll}}
	res, err := c.ReadProperties(testCtx(t), dev.device(), types.ReadMultipleProperty{Objects: []types.Object{
		{ID: avObject(1), Properties: props},
		{ID: avObject(2), Properties: props},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Objects) != 2 {
		t.Fatalf("got %d objects, want 2", len(res.Objects))
	}
	for _, obj := range res.Objects {
		if len(obj.Properties) != len(props) {
			t.Fatalf("%v: got %d properties, want %d", obj.ID, len(obj.Properties), len(props))
		}
		for i, p := range obj.Properties {
			if p.ID != props[i].ID || p.Data != values[obj.ID][p.ID] {
				t.Errorf("%v property %d: got %v=%v, want %v=%v", obj.ID, i, p.ID, p.Data, props[i].ID, values[obj.ID][props[i].ID])
			}
		}
	}
}

func TestClient_RemoteDevices_rpmAborted(t *testing.T) {
	devID := types.ObjectID{Type: objecttype.Device, Instance: 7}
	values := map[types.ObjectID]map[property.ID]interface{}{devID: {
		property.MaxApduLengthAccepted: uint32(480),
		property.SegmentationSupported: types.Enumerated(3),
		property.VendorIdentifier:      uint32(42),
	}}
	dev := startFakeDevice(t, func(_ *net.UDPAddr, req types.APDU) []byte {
		if req.Service == types.ServiceConfirmedReadPropMultiple {
			return abortAPDU(t, req, types.AbortSegmentationNotSupported)
		}
		return readPropertyAck(t, req, values)
	})
	c := newTestClient(t)

	devs, err := c.RemoteDevices(testCtx(t), types.UDPToAddress(dev.addr), devID.Instance)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].MaxApdu != 480 || devs[0].Segmentation != 3 || devs[0].Vendor != 42 {
		t.Fatalf("unexpected devices %+v", devs)
	}
}

func TestClient_ReadProperty_lostReply(t *testing.T) {
	dev := startFakeDevice(t, func(*net.UDPAddr, types.APDU) []byte { return nil })
	c := newTestClient(t, WithRequestTimeout(200*time.Millisecond))

	done := make(chan error, 1)
	go func() {
		_, err := readPV(context.Background(), c, dev.device(), avObject(1))
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got error %v, want a context.DeadlineExceeded", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadProperty did not time out")
	}
	if n := dev.requestCount(types.ServiceConfirmedReadProperty, 2, time.Second); n != 2 {
		t.Fatalf("device saw %d ReadProperty requests, want 2", n)
	}
}

func TestClient_ReadProperty_ignoresStrayReply(t *testing.T) {
	values := map[types.ObjectID]map[property.ID]interface{}{avObject(1): {property.PresentValue: float32(1)}}
	stray := listenLoopback(t)
	dev := startFakeDevice(t, func(from *net.UDPAddr, req types.APDU) []byte {
		// another device answers first, with the same invoke id
		wrong := map[types.ObjectID]map[property.ID]interface{}{avObject(1): {property.PresentValue: float32(99)}}
		sendAPDU(t, stray, from, readPropertyAck(t, req, wrong))
		time.Sleep(100 * time.Millisecond)
		return readPropertyAck(t, req, values)
	})
	c := newTestClient(t)

	res, err := readPV(testCtx(t), c, dev.device(), avObject(1))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Object.Properties[0].Data; got != float32(1) {
		t.Fatalf("got %v, want the value from the device the request went to", got)
	}
}
