// Copyright (c) 2019 Kien Nguyen-Tuan <kiennt2609@gmail.com>
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

package utils

import (
	"context"
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"hash/fnv"
	"net"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	etcdv3 "go.etcd.io/etcd/clientv3"
)

// WatchContext returns a cancelable context and cancel function
func WatchContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(etcdv3.WithRequireLeader(context.Background()))
}

// HashFNV generates a new 64-bit number from a given string
// using 64-bit FNV-1a hash function.
func HashFNV(s string) string {
	h := fnv.New64a()
	h.Write([]byte(s))
	return string(h.Sum64())
}

// Hash generates a new slice of bytee hash from a given string
// using a given hash algorithms.
func Hash(s string, f crypto.Hash) string {
	var h hash.Hash
	switch f {
	case crypto.MD5:
		h = md5.New()
	case crypto.SHA1:
		h = sha1.New()
	case crypto.SHA256:
		h = sha256.New()
	case crypto.SHA512:
		h = sha512.New()
	default:
	}
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// RandToken generates a random 16-bit token
func RandToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// Path returns a etcd key path.
func Path(keys ...string) string {
	return strings.Join(append([]string{}, keys...), "/")
}

// Secret special type for storing secrets.
type Secret string

// MarshalYAML implements the yaml.Marshaler interface for Secrets.
func (s Secret) MarshalYAML() (interface{}, error) {
	if s != "" {
		return "<secret>", nil
	}
	return nil, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Secrets.
func (s *Secret) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Secret
	return unmarshal((*plain)(s))
}

func (s Secret) MarshalJSON() ([]byte, error) {
	if s != "" {
		return []byte(`"<secret>"`), nil
	}
	return nil, nil
}

// Find tells whether string contains x.
// op - boolean operator, expected `AND` `OR` string value.
// x - could be string or slice of string.
func Find(a []string, x interface{}, op string) bool {
	var r bool
	switch reflect.TypeOf(x).Kind() {
	case reflect.String:
		for _, n := range a {
			if x == n {
				r = true
				break
			}
		}
	case reflect.Slice:
		v := reflect.ValueOf(x)
		for i := 0; i < v.Len(); i++ {
			r = Find(a, v.Index(i).String(), op)
			// If operator is OR, break the loop immediately when found the first match
			if strings.ToLower(op) == "or" && r {
				break
			}
		}
	}
	return r
}

// AddParts returns the parts of the address
func AddParts(address string) (string, int, error) {
	_, _, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}

	// Get the address
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return "", 0, err
	}

	return addr.IP.String(), addr.Port, nil
}

// RuntimeStats is used to return various runtime information
func RuntimeStats() map[string]string {
	return map[string]string{
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"version":    runtime.Version(),
		"max_procs":  strconv.FormatInt(int64(runtime.GOMAXPROCS(0)), 10),
		"goroutines": strconv.FormatInt(int64(runtime.NumGoroutine()), 10),
		"cpu_count":  strconv.FormatInt(int64(runtime.NumCPU()), 10),
	}
}

// ExternalIP returns an external ip address of the current host
func ExternalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("a node didn't connect to any networks")
}

// RetryableError determines that given error is retryable or not.
func RetryableError(err error) bool {
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	switch t := err.(type) {
	case *net.OpError:
		if t.Op == "dial" {
			return false
		}
		if t.Op == "read" {
			// Accept retry if there is connection refused error
			return true
		}
	case syscall.Errno:
		if t == syscall.ECONNREFUSED {
			// Accept retry if there is connection refused error
			return true
		}
	}
	return false
}
