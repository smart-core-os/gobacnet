package server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/smart-core-os/gobacnet"
	"github.com/smart-core-os/gobacnet/encoding"
	"github.com/smart-core-os/gobacnet/enum/engineeringunit"
	"github.com/smart-core-os/gobacnet/enum/errorcode"
	"github.com/smart-core-os/gobacnet/enum/pdutype"
	"github.com/smart-core-os/gobacnet/enum/reliability"
	"github.com/smart-core-os/gobacnet/property"
	"github.com/smart-core-os/gobacnet/types"
	"github.com/smart-core-os/gobacnet/types/objecttype"
)

const testInstance = 4001

var testDeviceID = types.ObjectID{Type: objecttype.Device, Instance: testInstance}

func avID(i types.ObjectInstance) types.ObjectID {
	return types.ObjectID{Type: objecttype.AnalogValue, Instance: i}
}

type testServer struct {
	*Server
	addr *net.UDPAddr
	temp *AnalogValue
	rh   *AnalogValue
}

func startServer(t *testing.T, opts ...Option) testServer {
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	opts = append([]Option{WithLogger(log)}, opts...)
	s := New(DeviceConfig{
		Instance:   testInstance,
		Name:       "Test Device",
		VendorID:   999,
		VendorName: "Test Vendor",
		ModelName:  "Test Model",
	}, opts...)

	ts := testServer{
		Server: s,
		temp:   NewAnalogValue(1, "Air Temp", engineeringunit.DegreesCelsius),
		rh:     NewAnalogValue(2, "Humidity", engineeringunit.PercentRelativeHumidity),
	}
	ts.temp.SetPresentValue(12.5)
	ts.temp.SetDescription("outside air temperature")
	ts.rh.SetPresentValue(80)
	for _, o := range []Object{ts.temp, ts.rh} {
		if err := s.AddObject(o); err != nil {
			t.Fatal(err)
		}
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	ts.addr = conn.LocalAddr().(*net.UDPAddr)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, conn) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	})
	return ts
}

// newClient returns the gobacnet client, as used by drivers reading from BACnet devices.
func newClient(t *testing.T) *gobacnet.Client {
	t.Helper()
	free, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		t.Fatal(err)
	}
	port := free.LocalAddr().(*net.UDPAddr).Port
	_ = free.Close()

	c, err := gobacnet.NewClient(ipv4Interface(t), port, gobacnet.WithLogLevel(logrus.WarnLevel))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// ipv4Interface finds an interface the client will accept: it needs one with an IPv4 address.
func ipv4Interface(t *testing.T) string {
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

func (ts testServer) device() types.Device {
	return types.Device{ID: testDeviceID, Addr: types.UDPToAddress(ts.addr), MaxApdu: encoding.MaxAPDU}
}

func testContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func readProp(t *testing.T, c *gobacnet.Client, dev types.Device, obj types.ObjectID, id property.ID, index uint32) (interface{}, error) {
	t.Helper()
	res, err := c.ReadProperty(testContext(t), dev, types.ReadPropertyData{Object: types.Object{
		ID:         obj,
		Properties: []types.Property{{ID: id, ArrayIndex: index}},
	}})
	if err != nil {
		return nil, err
	}
	return res.Object.Properties[0].Data, nil
}

func mustReadProp(t *testing.T, c *gobacnet.Client, dev types.Device, obj types.ObjectID, id property.ID, index uint32) interface{} {
	t.Helper()
	v, err := readProp(t, c, dev, obj, id, index)
	if err != nil {
		t.Fatalf("read %v %v[%d]: %v", obj, id, index, err)
	}
	return v
}

func TestServer_RemoteDevices(t *testing.T) {
	ts := startServer(t)
	c := newClient(t)

	devs, err := c.RemoteDevices(testContext(t), types.UDPToAddress(ts.addr), testInstance)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].Vendor != 999 || devs[0].MaxApdu != encoding.MaxAPDU || devs[0].Segmentation != segmentationNone {
		t.Fatalf("unexpected devices %+v", devs)
	}
}

func TestServer_ReadProperty(t *testing.T) {
	ts := startServer(t)
	c := newClient(t)
	dev := ts.device()

	tests := []struct {
		obj   types.ObjectID
		prop  property.ID
		index uint32
		want  interface{}
	}{
		{avID(1), property.PresentValue, encoding.ArrayAll, float32(12.5)},
		{avID(1), property.ObjectName, encoding.ArrayAll, "Air Temp"},
		{avID(1), property.Description, encoding.ArrayAll, "outside air temperature"},
		{avID(1), property.ObjectIdentifier, encoding.ArrayAll, avID(1)},
		{avID(2), property.Units, encoding.ArrayAll, uint32(engineeringunit.PercentRelativeHumidity)},
		{avID(2), property.OutOfService, encoding.ArrayAll, false},
		{avID(2), property.PropertyList, 0, uint32(7)},
		{testDeviceID, property.ObjectName, encoding.ArrayAll, "Test Device"},
		{testDeviceID, property.VendorIdentifier, encoding.ArrayAll, uint32(999)},
		{testDeviceID, property.ObjectList, 0, uint32(3)},
		{testDeviceID, property.ObjectList, 1, testDeviceID},
		{testDeviceID, property.ObjectList, 3, avID(2)},
		{testDeviceID, property.DatabaseRevision, encoding.ArrayAll, uint32(2)},
		// the wildcard device instance addresses this device
		{types.ObjectID{Type: objecttype.Device, Instance: types.MaxInstance}, property.ObjectIdentifier, encoding.ArrayAll, testDeviceID},
	}
	for _, tt := range tests {
		if got := mustReadProp(t, c, dev, tt.obj, tt.prop, tt.index); got != tt.want {
			t.Errorf("%v %v[%d] = %v (%T), want %v (%T)", tt.obj, tt.prop, tt.index, got, got, tt.want, tt.want)
		}
	}

	list, ok := mustReadProp(t, c, dev, testDeviceID, property.ObjectList, encoding.ArrayAll).([]interface{})
	if !ok || len(list) != 3 || list[0] != testDeviceID || list[1] != avID(1) || list[2] != avID(2) {
		t.Errorf("object list = %v", list)
	}

	// values set after serving starts are what gets read
	ts.temp.SetPresentValue(14)
	ts.temp.SetReliability(reliability.CommunicationFailure)
	if got := mustReadProp(t, c, dev, avID(1), property.PresentValue, encoding.ArrayAll); got != float32(14) {
		t.Errorf("present value after update = %v", got)
	}
	if got := mustReadProp(t, c, dev, avID(1), property.Reliability, encoding.ArrayAll); got != uint32(reliability.CommunicationFailure) {
		t.Errorf("reliability after update = %v", got)
	}
	if got := mustReadProp(t, c, dev, avID(1), property.StatusFlags, encoding.ArrayAll).(types.BitString); !got.At(1) || got.At(3) {
		t.Errorf("status flags after update = %v, want fault only", got)
	}
}

func TestServer_ReadProperty_errors(t *testing.T) {
	ts := startServer(t)
	c := newClient(t)

	tests := []struct {
		name  string
		obj   types.ObjectID
		prop  property.ID
		index uint32
		want  types.Error
	}{
		{"unknown object", avID(99), property.PresentValue, encoding.ArrayAll, ErrUnknownObject},
		{"other device", types.ObjectID{Type: objecttype.Device, Instance: 1}, property.ObjectName, encoding.ArrayAll, ErrUnknownObject},
		{"unknown property", avID(1), property.Polarity, encoding.ArrayAll, ErrUnknownProperty},
		{"all", avID(1), property.All, encoding.ArrayAll, ErrUnknownProperty},
		{"not an array", avID(1), property.PresentValue, 1, ErrPropertyIsNotAnArray},
		{"index out of range", testDeviceID, property.ObjectList, 4, ErrInvalidArrayIndex},
		{"list is not an array", testDeviceID, property.DeviceAddressBinding, 1, ErrPropertyIsNotAnArray},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := readProp(t, c, ts.device(), tt.obj, tt.prop, tt.index)
			var got types.Error
			if !errors.As(err, &got) || got != tt.want {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestServer_ReadPropertyMultiple(t *testing.T) {
	ts := startServer(t)
	c := newClient(t)

	props := func(ids ...property.ID) []types.Property {
		var ps []types.Property
		for _, id := range ids {
			ps = append(ps, types.Property{ID: id, ArrayIndex: encoding.ArrayAll})
		}
		return ps
	}
	// ReadMultiProperty, not ReadProperties, which falls back to ReadProperty on failure
	res, err := c.ReadMultiProperty(testContext(t), ts.device(), types.ReadMultipleProperty{Objects: []types.Object{
		{ID: avID(1), Properties: props(property.PresentValue, property.Units, property.ObjectName)},
		{ID: avID(2), Properties: props(property.PresentValue)},
		{ID: testDeviceID, Properties: props(property.MaxApduLengthAccepted, property.SegmentationSupported)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]interface{}{
		{float32(12.5), uint32(engineeringunit.DegreesCelsius), "Air Temp"},
		{float32(80)},
		{uint32(encoding.MaxAPDU), uint32(segmentationNone)},
	}
	if len(res.Objects) != len(want) {
		t.Fatalf("got %d objects, want %d", len(res.Objects), len(want))
	}
	for i, obj := range res.Objects {
		if len(obj.Properties) != len(want[i]) {
			t.Fatalf("object %v: got %d properties, want %d", obj.ID, len(obj.Properties), len(want[i]))
		}
		for j, p := range obj.Properties {
			if p.Data != want[i][j] {
				t.Errorf("object %v property %v = %v, want %v", obj.ID, p.ID, p.Data, want[i][j])
			}
		}
	}
}

func TestServer_ReadPropertyMultiple_all(t *testing.T) {
	ts := startServer(t)
	c := newClient(t)

	res, err := c.ReadMultiProperty(testContext(t), ts.device(), types.ReadMultipleProperty{Objects: []types.Object{
		{ID: avID(1), Properties: []types.Property{{ID: property.All, ArrayIndex: encoding.ArrayAll}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := res.Objects[0].Properties
	want := ts.temp.PropertyIDs()
	if len(got) != len(want) {
		t.Fatalf("got %d properties, want %d", len(got), len(want))
	}
	for i, p := range got {
		if p.ID != want[i] {
			t.Errorf("property %d = %v, want %v", i, p.ID, want[i])
		}
	}
}

func TestServer_ReadPropertyMultiple_partialFailure(t *testing.T) {
	ts := startServer(t)
	c := newClient(t)

	_, err := c.ReadMultiProperty(testContext(t), ts.device(), types.ReadMultipleProperty{Objects: []types.Object{
		{ID: avID(1), Properties: []types.Property{
			{ID: property.PresentValue, ArrayIndex: encoding.ArrayAll},
			{ID: property.Polarity, ArrayIndex: encoding.ArrayAll},
		}},
		{ID: avID(99), Properties: []types.Property{{ID: property.PresentValue, ArrayIndex: encoding.ArrayAll}}},
	}})
	var propErrs []types.PropertyAccessError
	for _, e := range unwrapAll(err) {
		var pe types.PropertyAccessError
		if errors.As(e, &pe) {
			propErrs = append(propErrs, pe)
		}
	}
	if len(propErrs) != 2 ||
		propErrs[0].Property != property.Polarity || propErrs[0].Err.Code != errorcode.UnknownProperty ||
		propErrs[1].ObjectID != avID(99) || propErrs[1].Err.Code != errorcode.UnknownObject {
		t.Fatalf("got %v", err)
	}
}

func unwrapAll(err error) []error {
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		return multi.Unwrap()
	}
	if multi, ok := err.(interface{ Errors() []error }); ok {
		return multi.Errors()
	}
	return []error{err}
}

// The tests below send raw packets, to cover what the client cannot send or decode.

type rawConn struct {
	t    *testing.T
	conn *net.UDPConn
}

func dial(t *testing.T, ts testServer) rawConn {
	t.Helper()
	conn, err := net.DialUDP("udp4", nil, ts.addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return rawConn{t: t, conn: conn}
}

func (r rawConn) send(bvlcFunc types.BacFunc, npdu types.NPDU, apdu []byte) {
	r.t.Helper()
	sendRaw(r.t, r.conn, nil, bvlcFunc, nil, npdu, apdu)
}

func sendRaw(t *testing.T, conn *net.UDPConn, to *net.UDPAddr, bvlcFunc types.BacFunc, forwardedFrom []byte, npdu types.NPDU, apdu []byte) {
	t.Helper()
	e := encoding.NewEncoder()
	e.NPDU(npdu)
	payload := append(forwardedFrom, append(e.Bytes(), apdu...)...)
	e = encoding.NewEncoder()
	_ = e.BVLC(types.BVLC{Type: types.BVLCTypeBacnetIP, Function: bvlcFunc, Length: uint16(4 + len(payload)), Data: payload})
	var err error
	if to == nil {
		_, err = conn.Write(e.Bytes())
	} else {
		_, err = conn.WriteToUDP(e.Bytes(), to)
	}
	if err != nil {
		t.Fatal(err)
	}
}

type response struct {
	bvlcFunc types.BacFunc
	npdu     types.NPDU
	apdu     types.APDU
}

func receive(t *testing.T, conn *net.UDPConn, wait time.Duration) (response, bool) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	buf := make([]byte, 2048)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return response{}, false
	}
	var r response
	r.bvlcFunc = types.BacFunc(buf[1])
	d := encoding.NewDecoder(buf[4:n])
	if err := d.NPDU(&r.npdu); err != nil {
		t.Fatal(err)
	}
	if err := d.APDU(&r.apdu); err != nil {
		t.Fatal(err)
	}
	return r, true
}

func (r rawConn) receive() response {
	r.t.Helper()
	resp, ok := receive(r.t, r.conn, 2*time.Second)
	if !ok {
		r.t.Fatal("no response")
	}
	return resp
}

func (r rawConn) expectNothing() {
	r.t.Helper()
	if resp, ok := receive(r.t, r.conn, 300*time.Millisecond); ok {
		r.t.Fatalf("unexpected response %+v", resp)
	}
}

var request = types.NPDU{Version: types.ProtocolVersion, ExpectingReply: true}

// whoIs returns a Who-Is APDU. Encoder.WhoIs writes the APDU header too.
func whoIs(t *testing.T, low, high int32) []byte {
	e := encoding.NewEncoder()
	if err := e.WhoIs(low, high); err != nil {
		t.Fatal(err)
	}
	return e.Bytes()
}

func decodeIAm(t *testing.T, r response) types.IAm {
	t.Helper()
	if r.apdu.DataType != pdutype.UnconfirmedServiceRequest || r.apdu.UnconfirmedService != types.ServiceUnconfirmedIAm {
		t.Fatalf("want I-Am, got %+v", r.apdu)
	}
	var iam types.IAm
	if err := encoding.NewDecoder(r.apdu.RawData).IAm(&iam); err != nil {
		t.Fatal(err)
	}
	return iam
}

func TestServer_WhoIs(t *testing.T) {
	ts := startServer(t)
	c := dial(t, ts)

	for _, rng := range [][2]int32{{types.WhoIsAll, types.WhoIsAll}, {testInstance, testInstance}, {4000, 5000}} {
		c.send(types.BacFuncBroadcast, types.NPDU{Version: types.ProtocolVersion}, whoIs(t, rng[0], rng[1]))
		iam := decodeIAm(t, c.receive())
		if iam.ID != testDeviceID || iam.MaxApdu != encoding.MaxAPDU || iam.Vendor != 999 || iam.Segmentation != segmentationNone {
			t.Fatalf("Who-Is %v: unexpected I-Am %+v", rng, iam)
		}
	}
	for _, rng := range [][2]int32{{1, 10}, {4002, 5000}} {
		c.send(types.BacFuncBroadcast, types.NPDU{Version: types.ProtocolVersion}, whoIs(t, rng[0], rng[1]))
		c.expectNothing()
	}
}

func TestServer_broadcastIAm(t *testing.T) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ts := startServer(t, WithBroadcastAddress(listener.LocalAddr().(*net.UDPAddr)))

	// announced at start-up
	r, ok := receive(t, listener, 2*time.Second)
	if !ok {
		t.Fatal("no I-Am at start-up")
	}
	if r.bvlcFunc != types.BacFuncBroadcast || decodeIAm(t, r).ID != testDeviceID {
		t.Fatalf("unexpected announcement %+v", r)
	}

	// and in answer to Who-Is, alongside the direct reply
	c := dial(t, ts)
	c.send(types.BacFuncBroadcast, types.NPDU{Version: types.ProtocolVersion}, whoIs(t, types.WhoIsAll, types.WhoIsAll))
	if r := c.receive(); r.bvlcFunc != types.BacFuncUnicast || decodeIAm(t, r).ID != testDeviceID {
		t.Fatalf("unexpected direct I-Am %+v", r)
	}
	if r, ok := receive(t, listener, 2*time.Second); !ok || r.bvlcFunc != types.BacFuncBroadcast {
		t.Fatalf("no broadcast I-Am after Who-Is")
	}
}

func readPropertyRequest(t *testing.T, invokeID uint8, obj types.ObjectID, id property.ID) []byte {
	e := encoding.NewEncoder()
	err := e.ReadProperty(invokeID, types.ReadPropertyData{Object: types.Object{
		ID:         obj,
		Properties: []types.Property{{ID: id, ArrayIndex: encoding.ArrayAll}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return e.Bytes()
}

func TestServer_rejectsUnsupportedServices(t *testing.T) {
	ts := startServer(t)
	c := dial(t, ts)

	e := encoding.NewEncoder()
	err := e.WriteProperty(5, types.ReadPropertyData{Object: types.Object{
		ID:         avID(1),
		Properties: []types.Property{{ID: property.PresentValue, ArrayIndex: encoding.ArrayAll, Data: float32(1)}},
	}}, 16)
	if err != nil {
		t.Fatal(err)
	}
	c.send(types.BacFuncUnicast, request, e.Bytes())
	r := c.receive()
	if r.apdu.DataType != pdutype.Reject || r.apdu.InvokeId != 5 || r.apdu.Error.Code != errorcode.RejectUnrecognizedService {
		t.Fatalf("want reject unrecognized-service, got %+v", r.apdu)
	}
}

func TestServer_abortsOversizeResponses(t *testing.T) {
	ts := startServer(t)
	for i := types.ObjectInstance(10); i < 30; i++ {
		if err := ts.AddObject(NewAnalogValue(i, "filler", engineeringunit.NoUnits)); err != nil {
			t.Fatal(err)
		}
	}
	c := dial(t, ts)

	req := readPropertyRequest(t, 6, testDeviceID, property.ObjectList)
	req[1] = 0x00 // max APDU accepted: 50 bytes, too small for 23 object identifiers
	c.send(types.BacFuncUnicast, request, req)
	r := c.receive()
	if r.apdu.DataType != pdutype.Abort || !r.apdu.Server || r.apdu.InvokeId != 6 || r.apdu.Error.Code != errorcode.AbortSegmentationNotSupported {
		t.Fatalf("want abort segmentation-not-supported, got %+v", r.apdu)
	}
}

func TestServer_forwardedNPDU(t *testing.T) {
	ts := startServer(t)

	// the BBMD forwards a request from origin, which is where the answer goes
	bbmd, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer bbmd.Close()
	origin, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer origin.Close()
	originAddr := origin.LocalAddr().(*net.UDPAddr)
	from := append(append([]byte{}, originAddr.IP.To4()...), byte(originAddr.Port>>8), byte(originAddr.Port))

	sendRaw(t, bbmd, ts.addr, types.BacFuncForwardedNPDU, from, request, readPropertyRequest(t, 3, avID(1), property.PresentValue))
	r, ok := receive(t, origin, 2*time.Second)
	if !ok {
		t.Fatal("no response at the original source")
	}
	if r.apdu.DataType != pdutype.ComplexAck || r.apdu.InvokeId != 3 {
		t.Fatalf("unexpected response %+v", r.apdu)
	}
	if _, ok := receive(t, bbmd, 100*time.Millisecond); ok {
		t.Fatal("response went to the BBMD")
	}
}

func TestServer_routedRequest(t *testing.T) {
	ts := startServer(t)
	c := dial(t, ts)

	// a request a router forwarded from MAC 0x0A on network 5
	routed := request
	routed.Source = &types.Address{Net: 5, Len: 1, Adr: []byte{0x0A}}
	c.send(types.BacFuncUnicast, routed, readPropertyRequest(t, 2, avID(2), property.PresentValue))
	r := c.receive()
	if d := r.npdu.Destination; d == nil || d.Net != 5 || len(d.Adr) != 1 || d.Adr[0] != 0x0A {
		t.Fatalf("reply not routed back to the source network: %+v", r.npdu)
	}
	if r.apdu.DataType != pdutype.ComplexAck || r.apdu.InvokeId != 2 {
		t.Fatalf("unexpected response %+v", r.apdu)
	}
}

func TestServer_AddObject(t *testing.T) {
	s := New(DeviceConfig{Instance: 1})
	if err := s.AddObject(NewAnalogValue(1, "a", engineeringunit.NoUnits)); err != nil {
		t.Fatal(err)
	}
	if err := s.AddObject(NewAnalogValue(1, "b", engineeringunit.NoUnits)); err == nil {
		t.Error("added a duplicate object")
	}
	if err := s.AddObject(&device{cfg: DeviceConfig{Instance: 2}}); err == nil {
		t.Error("added a second device")
	}
}
