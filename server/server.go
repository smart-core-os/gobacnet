package server

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/smart-core-os/gobacnet/encoding"
	"github.com/smart-core-os/gobacnet/enum/errorclass"
	"github.com/smart-core-os/gobacnet/enum/errorcode"
	"github.com/smart-core-os/gobacnet/enum/pdutype"
	"github.com/smart-core-os/gobacnet/property"
	"github.com/smart-core-os/gobacnet/types"
	"github.com/smart-core-os/gobacnet/types/objecttype"
)

const (
	bvlcHeaderLength      = 4
	forwardedHeaderLength = 10 // BVLC header plus the original source B/IP address
)

// Server is a BACnet/IP device. Create one with New, add objects with AddObject, then Serve.
type Server struct {
	dev       *device
	log       logrus.FieldLogger
	broadcast *net.UDPAddr

	mu      sync.RWMutex
	objects []Object // in Object_List order, after the device
	byID    map[types.ObjectID]Object
	dbRev   uint32

	connMu sync.Mutex
	conn   *net.UDPConn
}

type Option func(*Server)

// WithLogger sets the logger. By default the standard logrus logger is used.
func WithLogger(l logrus.FieldLogger) Option {
	return func(s *Server) {
		s.log = l
	}
}

// WithBroadcastAddress sets where I-Am is broadcast, for example 192.168.1.255:47808.
// When set, the server announces itself when Serve starts and broadcasts I-Am in answer to
// Who-Is, as well as answering the requester directly.
func WithBroadcastAddress(addr *net.UDPAddr) Option {
	return func(s *Server) {
		s.broadcast = addr
	}
}

// New returns a Server for the device described by cfg.
func New(cfg DeviceConfig, opts ...Option) *Server {
	s := &Server{
		log:  logrus.StandardLogger(),
		byID: map[types.ObjectID]Object{},
	}
	s.dev = &device{cfg: cfg, srv: s}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// AddObject adds an object to the device. It can be called before or while serving.
func (s *Server) AddObject(o Object) error {
	id := o.ObjectID()
	if id.Type == objecttype.Device {
		return fmt.Errorf("the server already has a device object")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[id]; ok {
		return fmt.Errorf("object %v:%d already exists", id.Type, id.Instance)
	}
	s.objects = append(s.objects, o)
	s.byID[id] = o
	s.dbRev++
	return nil
}

func (s *Server) objectList() Array {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := Array{s.dev.ObjectID()}
	for _, o := range s.objects {
		list = append(list, o.ObjectID())
	}
	return list
}

func (s *Server) objectTypes() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[types.ObjectType]bool{objecttype.Device: true}
	ts := []int{int(objecttype.Device)}
	for _, o := range s.objects {
		if t := o.ObjectID().Type; !seen[t] {
			seen[t] = true
			ts = append(ts, int(t))
		}
	}
	return ts
}

func (s *Server) databaseRevision() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dbRev
}

// lookup returns the object with the given id, or nil. The device can also be addressed by
// the wildcard instance MaxInstance (clause 12.11.1).
func (s *Server) lookup(id types.ObjectID) Object {
	if id.Type == objecttype.Device {
		if id.Instance == s.dev.cfg.Instance || id.Instance == types.MaxInstance {
			return s.dev
		}
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byID[id]
}

// ListenAndServe listens on the UDP address addr, ":47808" for example, and serves until ctx is done.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, conn)
}

// Serve answers BACnet requests arriving on conn until ctx is done, then closes conn.
func (s *Server) Serve(ctx context.Context, conn *net.UDPConn) error {
	s.connMu.Lock()
	s.conn = conn
	s.connMu.Unlock()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	if s.broadcast != nil {
		s.sendIAm(s.broadcast, nil, true)
	}

	buf := make([]byte, 2048)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			s.log.Warnf("bacnet server: read failed: %v", err)
			continue
		}
		s.handle(src, buf[:n])
	}
}

// handle processes one BVLL message from src.
func (s *Server) handle(src *net.UDPAddr, b []byte) {
	if len(b) < bvlcHeaderLength || b[0] != types.BVLCTypeBacnetIP {
		return
	}
	replyTo := src
	var npdu []byte
	switch types.BacFunc(b[1]) {
	case types.BacFuncUnicast, types.BacFuncBroadcast:
		npdu = b[bvlcHeaderLength:]
	case types.BacFuncForwardedNPDU:
		// forwarded by a BBMD: answer the original source, whose address the BBMD included
		if len(b) < forwardedHeaderLength {
			return
		}
		replyTo = &net.UDPAddr{IP: net.IP(b[4:8]), Port: int(binary.BigEndian.Uint16(b[8:10]))}
		npdu = b[forwardedHeaderLength:]
	default:
		return // BBMD management messages are not for us
	}

	dec := encoding.NewDecoder(npdu)
	var n types.NPDU
	if err := dec.NPDU(&n); err != nil || n.IsNetworkLayerMessage {
		return
	}
	var apdu types.APDU
	if err := dec.APDU(&apdu); err != nil {
		return
	}

	// requests that came through a router are answered back through it
	var replyNet *types.Address
	if n.Source != nil && n.Source.Net != 0 {
		replyNet = n.Source
	}

	switch apdu.DataType {
	case pdutype.UnconfirmedServiceRequest:
		if apdu.UnconfirmedService == types.ServiceUnconfirmedWhoIs {
			s.whoIs(replyTo, replyNet, apdu.RawData)
		}
	case pdutype.ConfirmedServiceRequest:
		s.confirmed(replyTo, replyNet, apdu)
	}
}

func (s *Server) whoIs(replyTo *net.UDPAddr, replyNet *types.Address, data []byte) {
	var low, high int32
	if err := encoding.NewDecoder(data).WhoIs(&low, &high); err != nil {
		return
	}
	instance := int32(s.dev.cfg.Instance)
	if low != types.WhoIsAll && (instance < low || instance > high) {
		return
	}
	s.sendIAm(replyTo, replyNet, false)
	if s.broadcast != nil {
		s.sendIAm(s.broadcast, nil, true)
	}
}

func (s *Server) sendIAm(to *net.UDPAddr, viaNet *types.Address, broadcast bool) {
	e := encoding.NewEncoder()
	_ = e.APDU(types.APDU{DataType: pdutype.UnconfirmedServiceRequest, UnconfirmedService: types.ServiceUnconfirmedIAm})
	err := e.IAm(types.IAm{
		ID:           s.dev.ObjectID(),
		MaxApdu:      encoding.MaxAPDU,
		Segmentation: segmentationNone,
		Vendor:       s.dev.cfg.VendorID,
	})
	if err != nil {
		s.log.Errorf("bacnet server: encode I-Am: %v", err)
		return
	}
	s.send(to, viaNet, broadcast, e.Bytes())
}

func (s *Server) confirmed(replyTo *net.UDPAddr, replyNet *types.Address, apdu types.APDU) {
	if apdu.SegmentedMessage {
		s.send(replyTo, replyNet, false, abort(apdu.InvokeId, types.AbortSegmentationNotSupported))
		return
	}

	var resp []byte
	switch apdu.Service {
	case types.ServiceConfirmedReadProperty:
		resp = s.readProperty(apdu)
	case types.ServiceConfirmedReadPropMultiple:
		resp = s.readPropertyMultiple(apdu)
	default:
		resp = reject(apdu.InvokeId, types.RejectUnrecognizedService)
	}

	maxResp := int(apdu.MaxApdu)
	if maxResp == 0 || maxResp > encoding.MaxAPDU {
		maxResp = encoding.MaxAPDU
	}
	if len(resp) > maxResp {
		resp = abort(apdu.InvokeId, types.AbortSegmentationNotSupported)
	}
	s.send(replyTo, replyNet, false, resp)
}

func (s *Server) readProperty(apdu types.APDU) []byte {
	var req types.ReadPropertyData
	if err := encoding.NewDecoder(apdu.RawData).ReadProperty(&req); err != nil || len(req.Object.Properties) != 1 {
		return reject(apdu.InvokeId, types.RejectInvalidTag)
	}
	prop := req.Object.Properties[0]

	o := s.lookup(req.Object.ID)
	if o == nil {
		return errorPDU(apdu, ErrUnknownObject)
	}
	switch prop.ID {
	case property.All, propertyRequired, propertyOptional:
		// only meaningful to ReadPropertyMultiple
		return errorPDU(apdu, ErrUnknownProperty)
	}
	value, err := readProperty(o, prop.ID, prop.ArrayIndex)
	if err != nil {
		return errorPDU(apdu, bacnetError(err))
	}

	e := encoding.NewEncoder()
	err = e.ReadPropertyAck(apdu.InvokeId, types.ReadPropertyData{Object: types.Object{
		ID:         o.ObjectID(),
		Properties: []types.Property{{ID: prop.ID, ArrayIndex: prop.ArrayIndex, Data: value}},
	}})
	if err != nil {
		s.log.Errorf("bacnet server: encode %v of %v: %v", prop.ID, o.ObjectID(), err)
		return errorPDU(apdu, types.Error{Class: errorclass.Device, Code: errorcode.Other})
	}
	return e.Bytes()
}

func (s *Server) readPropertyMultiple(apdu types.APDU) []byte {
	var req types.ReadMultipleProperty
	if err := encoding.NewDecoder(apdu.RawData).ReadMultipleProperty(&req); err != nil {
		return reject(apdu.InvokeId, types.RejectInvalidTag)
	}

	var res types.ReadMultipleProperty
	for _, reqObj := range req.Objects {
		o := s.lookup(reqObj.ID)
		resObj := types.Object{ID: reqObj.ID}
		if o != nil {
			resObj.ID = o.ObjectID()
		}
		for _, prop := range reqObj.Properties {
			if o == nil {
				resObj.Properties = append(resObj.Properties, types.Property{ID: prop.ID, ArrayIndex: prop.ArrayIndex, Data: ErrUnknownObject})
				continue
			}
			for _, prop := range expand(o, prop) {
				value, err := readProperty(o, prop.ID, prop.ArrayIndex)
				if err != nil {
					value = bacnetError(err)
				}
				resObj.Properties = append(resObj.Properties, types.Property{ID: prop.ID, ArrayIndex: prop.ArrayIndex, Data: value})
			}
		}
		res.Objects = append(res.Objects, resObj)
	}

	e := encoding.NewEncoder()
	if err := e.ReadMultiplePropertyAck(apdu.InvokeId, res); err != nil {
		s.log.Errorf("bacnet server: encode ReadPropertyMultiple response: %v", err)
		return errorPDU(apdu, types.Error{Class: errorclass.Device, Code: errorcode.Other})
	}
	return e.Bytes()
}

// expand turns the ALL, REQUIRED and OPTIONAL special property identifiers into the object's
// properties. Property_List is not included (clause 15.7.3.1.2).
func expand(o Object, prop types.Property) []types.Property {
	switch prop.ID {
	case property.All, propertyRequired, propertyOptional:
		var props []types.Property
		for _, id := range o.PropertyIDs() {
			if id == property.PropertyList {
				continue
			}
			props = append(props, types.Property{ID: id, ArrayIndex: encoding.ArrayAll})
		}
		return props
	}
	return []types.Property{prop}
}

// readProperty reads a property of o and applies the array index, if any.
func readProperty(o Object, id property.ID, index uint32) (interface{}, error) {
	var value interface{}
	if id == property.PropertyList {
		value = propertyList(o)
	} else {
		var err error
		if value, err = o.ReadProperty(id); err != nil {
			return nil, err
		}
	}

	switch v := value.(type) {
	case Array:
		switch {
		case index == encoding.ArrayAll:
			return []interface{}(v), nil
		case index == 0:
			return uint32(len(v)), nil
		case uint64(index) > uint64(len(v)):
			return nil, ErrInvalidArrayIndex
		default:
			return v[index-1], nil
		}
	case List:
		if index != encoding.ArrayAll {
			return nil, ErrPropertyIsNotAnArray
		}
		return []interface{}(v), nil
	default:
		if index != encoding.ArrayAll {
			return nil, ErrPropertyIsNotAnArray
		}
		return v, nil
	}
}

// bacnetError returns the BACnet error to send for err.
func bacnetError(err error) types.Error {
	var bacErr types.Error
	if errors.As(err, &bacErr) {
		return bacErr
	}
	return types.Error{Class: errorclass.Device, Code: errorcode.Other}
}

func errorPDU(apdu types.APDU, bacErr types.Error) []byte {
	e := encoding.NewEncoder()
	_ = e.ErrorPDU(apdu.InvokeId, apdu.Service, bacErr)
	return e.Bytes()
}

func reject(invokeID uint8, reason types.RejectReason) []byte {
	e := encoding.NewEncoder()
	_ = e.RejectPDU(invokeID, reason)
	return e.Bytes()
}

func abort(invokeID uint8, reason types.AbortReason) []byte {
	e := encoding.NewEncoder()
	_ = e.AbortPDU(invokeID, true, reason)
	return e.Bytes()
}

// send wraps apdu in an NPDU and BVLC header and sends it to addr.
func (s *Server) send(to *net.UDPAddr, viaNet *types.Address, broadcast bool, apdu []byte) {
	npdu := types.NPDU{
		Version:     types.ProtocolVersion,
		Destination: viaNet,
		Priority:    types.Normal,
		HopCount:    types.DefaultHopCount,
	}
	e := encoding.NewEncoder()
	e.NPDU(npdu)
	payload := append(e.Bytes(), apdu...)

	header := types.BVLC{
		Type:     types.BVLCTypeBacnetIP,
		Function: types.BacFuncUnicast,
		Length:   uint16(bvlcHeaderLength + len(payload)),
		Data:     payload,
	}
	if broadcast {
		header.Function = types.BacFuncBroadcast
	}
	e = encoding.NewEncoder()
	if err := e.BVLC(header); err != nil {
		s.log.Errorf("bacnet server: encode BVLC: %v", err)
		return
	}

	s.connMu.Lock()
	conn := s.conn
	s.connMu.Unlock()
	if _, err := conn.WriteToUDP(e.Bytes(), to); err != nil {
		s.log.Warnf("bacnet server: send to %v: %v", to, err)
	}
}
