// Copyright 2018 xft. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.

package modbus

import (
	"io"
	"time"
)

// RTUOverTCPClientHandler implements Packager and Transporter interface.
type RTUOverTCPClientHandler struct {
	rtuPackager
	rtuTCPTransporter
}

// NewRTUOverTCPClientHandler allocates and initializes a RTUOverTCPClientHandler.
func NewRTUOverTCPClientHandler(address string) *RTUOverTCPClientHandler {
	handler := &RTUOverTCPClientHandler{}
	handler.Address = address
	handler.Timeout = tcpTimeout
	handler.IdleTimeout = tcpIdleTimeout
	return handler
}

// RTUOverTCPClient creates RTU over TCP client with default handler and given connect string.
func RTUOverTCPClient(address string) Client {
	handler := NewRTUOverTCPClientHandler(address)
	return NewClient(handler)
}

// rtuTCPTransporter implements Transporter interface.
type rtuTCPTransporter struct {
	tcpTransporter
}

// Send sends data to server and ensures adequate response for request type
func (mb *rtuTCPTransporter) Send(aduRequest []byte) (aduResponse []byte, err error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	// Establish a new connection if not connected
	if err = mb.connect(); err != nil {
		return
	}

	// clear read buf
	mb.conn.SetDeadline(time.Now().Add(100 * time.Microsecond))
	io.ReadAll(mb.conn)

	// Set timer to close when idle
	mb.lastActivity = time.Now()
	mb.startCloseTimer()
	// Set write and read timeout
	var timeout time.Time
	if mb.Timeout > 0 {
		timeout = mb.lastActivity.Add(mb.Timeout)
	}
	if err = mb.conn.SetDeadline(timeout); err != nil {
		return
	}

	// Send the request
	mb.logf("modbus: send % x\n", aduRequest)
	if _, err = mb.conn.Write(aduRequest); err != nil {
		return
	}

	aduResponse, err = mb.readResponse(aduRequest)
	return
}

func (mb *rtuTCPTransporter) readResponse(aduRequest []byte) (aduResponse []byte, err error) {
	address := aduRequest[0]
	function := aduRequest[1]
	functionFail := aduRequest[1] & 0x80
	bytesToRead := calculateResponseLength(aduRequest)

	var n int
	var n1 int
	var data [rtuMaxSize]byte

	for {
		//We first read the minimum length and then read either the full package
		//or the error package, depending on the error status (byte 2 of the response)
		n, err = io.ReadAtLeast(mb.conn, data[:], rtuMinSize)
		if err != nil {
			return
		}
		//if the function is correct
		if data[1] == function {
			//we read the rest of the bytes
			if n < bytesToRead {
				if bytesToRead > rtuMinSize && bytesToRead <= rtuMaxSize {
					n1, err = io.ReadFull(mb.conn, data[n:bytesToRead])
					n += n1
				}
			}
		} else if data[1] == functionFail {
			//for error we need to read 5 bytes
			if n < rtuExceptionSize {
				n1, err = io.ReadFull(mb.conn, data[n:rtuExceptionSize])
			}
			n += n1
		}

		if err != nil {
			return
		}
		aduResponse = data[:n]
		mb.logf("modbus: recv % x\n", aduResponse)
		if aduResponse[0] == address {
			return
		}
	}
}
