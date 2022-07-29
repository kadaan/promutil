package config

import (
	"fmt"
	"github.com/kadaan/promutil/lib/errors"
	"net"
	"strconv"
)

type ListenAddress struct {
	Host string
	Port int
}

func (l ListenAddress) String() string {
	return fmt.Sprintf("%s:%d", l.Host, l.Port)
}

type listenAddressValue ListenAddress

func NewListenAddressValue(p *ListenAddress, val ListenAddress) *listenAddressValue {
	*p = val
	return (*listenAddressValue)(p)
}

// String is used both by fmt.Print and by Cobra in help text
func (e *listenAddressValue) String() string {
	return ListenAddress(*e).String()
}

// Set must have pointer receiver, so it doesn't change the value of a copy
func (e *listenAddressValue) Set(v string) error {
	host, port, err := net.SplitHostPort(v)
	if err != nil {
		return errors.Wrap(err, "failed to split listenAddress")
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return errors.Wrap(err, "failed to parse port")
	}
	if 1 > portNum || portNum > 0xffff {
		return errors.New("port number out of range: %d", port)
	}
	*e = listenAddressValue(ListenAddress{Host: host, Port: portNum})
	return nil
}

// Type is only used in help text
func (e *listenAddressValue) Type() string {
	return "listenAddress"
}
