// Copyright 2015-2016, Cyrill @ Schumacher.fm and the CoreStore contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package caddyesi

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
)

type responseBufferWriter interface {
	http.ResponseWriter
	FlushHeader(addContentLength int)
}

// responseWrapBuffer wraps an http.ResponseWriter, returning a proxy which only writes
// into the provided io.Writer.
func responseWrapBuffer(buf io.Writer, w http.ResponseWriter) responseBufferWriter {
	_, cn := w.(http.CloseNotifier)
	_, fl := w.(http.Flusher)
	_, hj := w.(http.Hijacker)
	_, rf := w.(io.ReaderFrom)

	bw := bufferedWriter{
		rw:     w,
		buf:    buf,
		header: make(http.Header),
	}
	if cn && fl && hj && rf {
		return &bufferedFancyWriter{bw}
	}
	if fl {
		return &bufferedFlushWriter{bw}
	}
	return &bw
}

// bufferedWriter wraps a http.ResponseWriter that implements the minimal
// http.ResponseWriter interface.
type bufferedWriter struct {
	rw            http.ResponseWriter
	buf           io.Writer
	flushedHeader bool
	wroteHeader   bool
	code          int
	headerMu      sync.Mutex
	header        http.Header
}

func (b *bufferedWriter) FlushHeader(addContentLength int) {
	if b.flushedHeader {
		return
	}
	b.headerMu.Lock()
	defer b.headerMu.Unlock()

	const clname = "Content-Length"
	clRaw := b.header.Get(clname)
	cl, _ := strconv.Atoi(clRaw) // ignoring that err ... for now
	b.header.Set(clname, strconv.Itoa(cl+addContentLength))

	for k, v := range b.header {
		b.rw.Header()[k] = v
	}
	b.rw.WriteHeader(b.code)
	b.flushedHeader = true
}

func (b *bufferedWriter) Header() http.Header {
	return b.header
}

func (b *bufferedWriter) WriteHeader(code int) {
	if !b.wroteHeader {
		b.code = code
		b.wroteHeader = true
	}
}

// Write does not write to the client instead it writes in the underlying buffer.
func (b *bufferedWriter) Write(p []byte) (int, error) {
	return b.buf.Write(p)
}

// bufferedFancyWriter is a writer that additionally satisfies
// http.CloseNotifier, http.Flusher, http.Hijacker, and io.ReaderFrom. It exists
// for the common case of wrapping the http.ResponseWriter that package http
// gives you, in order to make the proxied object support the full method set of
// the proxied object.
type bufferedFancyWriter struct {
	bufferedWriter
}

func (f *bufferedFancyWriter) CloseNotify() <-chan bool {
	cn := f.bufferedWriter.rw.(http.CloseNotifier)
	return cn.CloseNotify()
}
func (f *bufferedFancyWriter) Flush() {
	fl := f.bufferedWriter.rw.(http.Flusher)
	fl.Flush()
}
func (f *bufferedFancyWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj := f.bufferedWriter.rw.(http.Hijacker)
	return hj.Hijack()
}
func (f *bufferedFancyWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := f.bufferedWriter.rw.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return nil
}

// ReadFrom writes r into the underlying buffer
func (f *bufferedFancyWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(&f.bufferedWriter, r)
}

var _ http.CloseNotifier = &bufferedFancyWriter{}
var _ http.Flusher = &bufferedFancyWriter{}
var _ http.Hijacker = &bufferedFancyWriter{}
var _ http.Pusher = &bufferedFancyWriter{}
var _ io.ReaderFrom = &bufferedFancyWriter{}
var _ http.Flusher = &bufferedFlushWriter{}

// bufferedFlushWriter implements only http.Flusher mostly used
type bufferedFlushWriter struct {
	bufferedWriter
}

func (f *bufferedFlushWriter) Flush() {
	fl := f.bufferedWriter.rw.(http.Flusher)
	fl.Flush()
}