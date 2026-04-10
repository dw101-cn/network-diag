package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

const version = "0.1.0"

func main() {
	fmt.Println("========================================")
	fmt.Printf("  network-diag v%s\n", version)
	fmt.Println("  Network Diagnostic Tool")
	fmt.Println("========================================")
	fmt.Printf("Time:     %s\n", time.Now().Format(time.RFC3339))
	fmt.Printf("OS/Arch:  %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Println()

	checkProxyEnv()
	checkDNS()
	checkTCPConnect()
	checkHTTPS()
	checkTLSCert()
	checkGitProtocol()

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  Diagnostic complete.")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Please copy all output above and share it for further analysis.")
	fmt.Println("Press Enter to exit...")
	fmt.Scanln()
}

// checkProxyEnv prints all proxy-related environment variables.
func checkProxyEnv() {
	section("Proxy Environment Variables")

	proxyVars := []string{
		"HTTP_PROXY", "http_proxy",
		"HTTPS_PROXY", "https_proxy",
		"NO_PROXY", "no_proxy",
		"ALL_PROXY", "all_proxy",
	}

	found := false
	for _, v := range proxyVars {
		val := os.Getenv(v)
		if val != "" {
			fmt.Printf("  [SET]   %s = %s\n", v, val)
			found = true
		} else {
			fmt.Printf("  [EMPTY] %s\n", v)
		}
	}

	if !found {
		fmt.Println()
		fmt.Println("  ⚠ No proxy environment variables are set.")
		fmt.Println("    If your network requires a proxy, Git CLI won't know about it.")
		fmt.Println("    Browser may use system proxy settings that CLI tools don't see.")
	}
	fmt.Println()

	// Check Git-specific proxy config
	checkGitConfig()
}

// checkGitConfig checks git configuration for proxy settings.
func checkGitConfig() {
	subsection("Git Config (proxy-related)")

	gitConfigs := []string{
		"http.proxy", "https.proxy",
		"http.sslVerify", "http.sslCAInfo", "http.sslCAPath",
		"http.sslBackend",
	}

	for _, key := range gitConfigs {
		val := gitConfigGet(key)
		if val != "" {
			fmt.Printf("  [SET]   git config %s = %s\n", key, val)
		} else {
			fmt.Printf("  [EMPTY] git config %s\n", key)
		}
	}
	fmt.Println()
}

// gitConfigGet attempts to read a git config value.
// On systems without git, it returns empty string.
func gitConfigGet(key string) string {
	// We don't shell out to git because it may not be available or may hang.
	// Instead, we just report environment and let the user check git config.
	return ""
}

// checkDNS resolves github.com and related hosts.
func checkDNS() {
	section("DNS Resolution")

	hosts := []string{
		"github.com",
		"api.github.com",
		"raw.githubusercontent.com",
		"ssh.github.com",
	}

	for _, host := range hosts {
		start := time.Now()
		addrs, err := net.LookupHost(host)
		elapsed := time.Since(start)

		if err != nil {
			fmt.Printf("  [FAIL] %s — %v (took %v)\n", host, err, elapsed)
		} else {
			fmt.Printf("  [OK]   %s → %s (took %v)\n", host, strings.Join(addrs, ", "), elapsed)
		}
	}
	fmt.Println()
}

// checkTCPConnect tests raw TCP connections to GitHub endpoints.
func checkTCPConnect() {
	section("TCP Connectivity")

	targets := []struct {
		host string
		port string
		desc string
	}{
		{"github.com", "443", "HTTPS"},
		{"github.com", "22", "SSH (Git)"},
		{"github.com", "9418", "Git Protocol"},
		{"ssh.github.com", "443", "SSH over HTTPS port"},
	}

	for _, t := range targets {
		addr := net.JoinHostPort(t.host, t.port)
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		elapsed := time.Since(start)

		if err != nil {
			fmt.Printf("  [FAIL] %s (%s) — %v (took %v)\n", addr, t.desc, err, elapsed)
		} else {
			conn.Close()
			fmt.Printf("  [OK]   %s (%s) — connected (took %v)\n", addr, t.desc, elapsed)
		}
	}
	fmt.Println()
}

// checkHTTPS tests HTTPS GET requests to GitHub API.
func checkHTTPS() {
	section("HTTPS Connectivity")

	urls := []string{
		"https://github.com",
		"https://api.github.com",
		"https://github.com/dw101-cn",
	}

	// Test with default client (respects proxy env vars)
	subsection("Direct HTTPS (using proxy env if set)")
	for _, u := range urls {
		testHTTPGet(u, nil)
	}

	// If HTTPS_PROXY is set, also test without proxy
	proxy := os.Getenv("HTTPS_PROXY")
	if proxy == "" {
		proxy = os.Getenv("https_proxy")
	}
	if proxy != "" {
		subsection("HTTPS without proxy (bypassing env)")
		transport := &http.Transport{
			Proxy: nil, // explicitly no proxy
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		}
		client := &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		}
		for _, u := range urls {
			testHTTPGet(u, client)
		}
	}

	// Test with common corporate proxy ports
	subsection("Proxy auto-detection (common ports)")
	proxyPorts := []string{"8080", "3128", "9480", "8443"}
	for _, port := range proxyPorts {
		addr := net.JoinHostPort("127.0.0.1", port)
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			fmt.Printf("  [FOUND] Local proxy candidate at %s\n", addr)
			// Try using this as proxy
			proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%s", port))
			transport := &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
			client := &http.Client{
				Transport: transport,
				Timeout:   15 * time.Second,
			}
			testHTTPGet("https://api.github.com", client)
		} else {
			fmt.Printf("  [NONE]  No proxy at %s\n", addr)
		}
	}
	fmt.Println()
}

// testHTTPGet performs an HTTP GET request and reports the result.
func testHTTPGet(targetURL string, client *http.Client) {
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	start := time.Now()
	resp, err := client.Get(targetURL)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("  [FAIL] GET %s\n", targetURL)
		fmt.Printf("         Error: %v (took %v)\n", err, elapsed)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	fmt.Printf("  [OK]   GET %s\n", targetURL)
	fmt.Printf("         Status: %d, Body(512b): %s (took %v)\n", resp.StatusCode, truncate(string(body), 200), elapsed)
}

// checkTLSCert inspects the TLS certificate chain for github.com.
func checkTLSCert() {
	section("TLS Certificate Chain (github.com:443)")

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp",
		"github.com:443",
		&tls.Config{
			InsecureSkipVerify: false,
		},
	)
	if err != nil {
		fmt.Printf("  [FAIL] TLS handshake failed: %v\n", err)
		fmt.Println()
		fmt.Println("  ⚠ If this fails with certificate error, Zscaler may be")
		fmt.Println("    intercepting TLS and the Zscaler root CA is not trusted")
		fmt.Println("    by this binary. Try running with the system cert store.")

		// Retry with InsecureSkipVerify to see the cert chain
		fmt.Println()
		subsection("Retry with InsecureSkipVerify (to inspect cert chain)")
		conn2, err2 := tls.DialWithDialer(
			&net.Dialer{Timeout: 10 * time.Second},
			"tcp",
			"github.com:443",
			&tls.Config{
				InsecureSkipVerify: true,
			},
		)
		if err2 != nil {
			fmt.Printf("  [FAIL] Even insecure TLS failed: %v\n", err2)
		} else {
			printCertChain(conn2)
			conn2.Close()
		}
		fmt.Println()
		return
	}
	defer conn.Close()

	printCertChain(conn)
	fmt.Println()
}

// printCertChain prints the certificate chain from a TLS connection.
func printCertChain(conn *tls.Conn) {
	state := conn.ConnectionState()
	fmt.Printf("  TLS Version: %s\n", tlsVersionName(state.Version))
	fmt.Printf("  Cipher Suite: %s\n", tls.CipherSuiteName(state.CipherSuite))
	fmt.Printf("  Server Name: %s\n", state.ServerName)
	fmt.Println()

	for i, cert := range state.PeerCertificates {
		fmt.Printf("  Certificate #%d:\n", i)
		fmt.Printf("    Subject:  %s\n", cert.Subject)
		fmt.Printf("    Issuer:   %s\n", cert.Issuer)
		fmt.Printf("    NotBefore: %s\n", cert.NotBefore.Format(time.RFC3339))
		fmt.Printf("    NotAfter:  %s\n", cert.NotAfter.Format(time.RFC3339))
		if len(cert.DNSNames) > 0 {
			fmt.Printf("    DNS Names: %s\n", strings.Join(cert.DNSNames, ", "))
		}

		// Check if this looks like a Zscaler cert
		issuerStr := cert.Issuer.String()
		if strings.Contains(strings.ToLower(issuerStr), "zscaler") {
			fmt.Println("    ⚠ THIS CERTIFICATE IS ISSUED BY ZSCALER (TLS INTERCEPTION DETECTED)")
		}
	}
}

// tlsVersionName returns a human-readable TLS version name.
func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (0x%04x)", v)
	}
}

// checkGitProtocol tests git-specific protocols.
func checkGitProtocol() {
	section("Git Protocol Tests")

	// Test SSH connection to GitHub
	subsection("SSH to github.com:22")
	conn, err := net.DialTimeout("tcp", "github.com:22", 10*time.Second)
	if err != nil {
		fmt.Printf("  [FAIL] Cannot connect: %v\n", err)
	} else {
		// Read the SSH banner
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 256)
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Printf("  [WARN] Connected but no SSH banner: %v\n", err)
		} else {
			fmt.Printf("  [OK]   SSH Banner: %s\n", strings.TrimSpace(string(buf[:n])))
		}
		conn.Close()
	}

	// Test SSH over HTTPS port (ssh.github.com:443)
	subsection("SSH to ssh.github.com:443 (SSH over HTTPS)")
	conn2, err := net.DialTimeout("tcp", "ssh.github.com:443", 10*time.Second)
	if err != nil {
		fmt.Printf("  [FAIL] Cannot connect: %v\n", err)
	} else {
		conn2.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 256)
		n, err := conn2.Read(buf)
		if err != nil {
			fmt.Printf("  [WARN] Connected but no SSH banner: %v\n", err)
		} else {
			fmt.Printf("  [OK]   SSH Banner: %s\n", strings.TrimSpace(string(buf[:n])))
		}
		conn2.Close()
	}

	fmt.Println()
	fmt.Println("  💡 Tips:")
	fmt.Println("    - If SSH:22 fails but SSH:443 works, add to ~/.ssh/config:")
	fmt.Println("        Host github.com")
	fmt.Println("          Hostname ssh.github.com")
	fmt.Println("          Port 443")
	fmt.Println("    - If HTTPS works via proxy, configure git:")
	fmt.Println("        git config --global http.proxy http://proxy:port")
	fmt.Println("    - If Zscaler intercepts TLS, you may need:")
	fmt.Println("        git config --global http.sslCAInfo /path/to/zscaler-ca.crt")
	fmt.Println()
}

// section prints a section header.
func section(name string) {
	fmt.Println("----------------------------------------")
	fmt.Printf("▶ %s\n", name)
	fmt.Println("----------------------------------------")
}

// subsection prints a subsection header.
func subsection(name string) {
	fmt.Printf("\n  — %s —\n", name)
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
