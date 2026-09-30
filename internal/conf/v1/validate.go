// Package conf contains the generated and validated runtime configuration.
package conf

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
)

const maximumTimeout = 30 * time.Second

func (c *Bootstrap) Validate() error {
	if c == nil {
		return fmt.Errorf("bootstrap config is required")
	}
	if c.Server == nil || c.Server.Grpc == nil || c.Server.Admin == nil {
		return fmt.Errorf("grpc and admin server config are required")
	}
	if c.Command != nil {
		for _, reference := range []string{c.Command.DatabaseUrlFile, c.Command.ClientCaFile, c.Command.CertificateFile, c.Command.PrivateKeyFile} {
			if !filepath.IsAbs(reference) || filepath.Clean(reference) != reference {
				return fmt.Errorf("command connection materials require absolute file references")
			}
		}
		if len(c.Command.GovernanceDnsName) > 253 || !commandDNSName.MatchString(c.Command.GovernanceDnsName) || c.Command.GovernanceDnsName == "unknown" {
			return fmt.Errorf("command requires an explicit exact Governance DNS identity")
		}
	}
	if err := validateListener("grpc", c.Server.Grpc.Network, c.Server.Grpc.Addr, c.Server.Grpc.Timeout); err != nil {
		return err
	}
	if err := validateListener("admin", c.Server.Admin.Network, c.Server.Admin.Addr, c.Server.Admin.Timeout); err != nil {
		return err
	}
	_, grpcPortText, _ := net.SplitHostPort(c.Server.Grpc.Addr)
	_, adminPortText, _ := net.SplitHostPort(c.Server.Admin.Addr)
	grpcPort, _ := strconv.Atoi(grpcPortText)
	adminPort, _ := strconv.Atoi(adminPortText)
	if grpcPort == adminPort {
		return fmt.Errorf("grpc and admin listeners must use distinct ports")
	}
	return validateDuration("shutdown", c.Server.ShutdownTimeout, maximumTimeout)
}

var commandDNSName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`)

func validateListener(name, network, address string, timeout *durationpb.Duration) error {
	if network != "tcp" {
		return fmt.Errorf("%s network must be tcp", name)
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%s address: %w", name, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || (!ip.IsLoopback() && !ip.IsUnspecified()) {
		return fmt.Errorf("%s address must use a literal loopback or unspecified IP", name)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("%s address requires a numeric port in 1..65535", name)
	}
	return validateDuration(name, timeout, maximumTimeout)
}

func validateDuration(name string, value *durationpb.Duration, maximum time.Duration) error {
	if value == nil {
		return fmt.Errorf("%s timeout is required", name)
	}
	if err := value.CheckValid(); err != nil {
		return fmt.Errorf("%s timeout: %w", name, err)
	}
	duration := value.AsDuration()
	if duration <= 0 || duration > maximum {
		return fmt.Errorf("%s timeout must be within 1ns..%s", name, maximum)
	}
	return nil
}
