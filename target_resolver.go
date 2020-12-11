// The MIT License
//
// Copyright (c) 2019-2020, Cloudflare, Inc. and Apple, Inc. All rights reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"fmt"
	"github.com/miekg/dns"
	"net"
	"time"
)

type targetResolver struct {
	nameserver string
	timeout    time.Duration
}

func (s targetResolver) getResolverServerName() string {
	return s.nameserver
}

func (s targetResolver) resolve(query *dns.Msg) (*dns.Msg, error) {
	connection := new(dns.Conn)
	var err error
	if connection.Conn, err = net.DialTimeout("tcp", s.nameserver, s.timeout*time.Millisecond); err != nil {
		return nil, fmt.Errorf("Failed starting resolver connection")
	}

	connection.SetReadDeadline(time.Now().Add(s.timeout * time.Millisecond))
	connection.SetWriteDeadline(time.Now().Add(s.timeout * time.Millisecond))

	if err := connection.WriteMsg(query); err != nil {
		return nil, err
	}

	response, err := connection.ReadMsg()
	if err != nil {
		return nil, err
	}

	response.Id = query.Id
	return response, nil
}
