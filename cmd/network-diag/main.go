package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const version = "0.2.0"

var output io.Writer = os.Stdout

// Diagnostic findings collected during checks, used in summary.
var (
	foundProxies    []string // proxy URLs discovered from PAC/registry/env
	workingProxy    string   // first proxy that can reach GitHub via HTTPS
	zscalerDetected bool     // PAC URL or process indicates Zscaler
	needCustomCert  bool     // proxy works only with InsecureSkipVerify

	dnsOK           bool
	tcpHTTPSOK      bool
	httpsDirectOK   bool
	httpsViaProxyOK bool
	tlsDirectOK     bool
	sshPort22OK     bool
	sshPort443OK    bool

	gitInstalled      bool
	gitHTTPSDirectOK  bool
	gitHTTPSProxyOK   bool
	gitSSHDirectOK    bool
	gitSSH443OK       bool
	sshConfigOK       bool
	sshKeyFound       bool
	sshKeyType        string
)

func main() {
	filename := fmt.Sprintf("network-diag_%s.txt", time.Now().Format("20060102_150405"))
	logFile, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot create log file: %v\n", err)
	} else {
		defer logFile.Close()
		output = io.MultiWriter(os.Stdout, logFile)
	}

	printf("========================================\n")
	printf("  network-diag v%s\n", version)
	printf("  Network Diagnostic Tool\n")
	printf("========================================\n")
	printf("Time:     %s\n", time.Now().Format(time.RFC3339))
	printf("OS/Arch:  %s/%s\n", runtime.GOOS, runtime.GOARCH)
	printf("\n")

	checkProxyEnv()
	checkSystemProxy()
	checkDNS()
	checkTCPConnect()
	checkHTTPSDirect()
	checkHTTPSViaProxy()
	checkTLSCert()
	checkGitSetup()
	checkGitOperations()
	checkSSHProtocol()
	printSummary()

	printf("\n")
	printf("Results saved to: %s\n", filename)
	printf("\n")
	fmt.Println("Press Enter to exit...")
	fmt.Scanln()
}

// ─── Output helpers ─────────────────────────────────────────

func printf(format string, a ...any) {
	fmt.Fprintf(output, format, a...)
}

func println(s string) {
	fmt.Fprintln(output, s)
}

func section(name string) {
	println("----------------------------------------")
	printf("# %s\n", name)
	println("----------------------------------------")
}

func subsection(name string) {
	printf("\n  -- %s --\n", name)
}

// ─── 1. Proxy Environment ───────────────────────────────────

func checkProxyEnv() {
	section("Proxy Environment Variables")

	proxyVars := []string{
		"HTTP_PROXY", "http_proxy",
		"HTTPS_PROXY", "https_proxy",
		"NO_PROXY", "no_proxy",
		"ALL_PROXY", "all_proxy",
	}

	for _, v := range proxyVars {
		val := os.Getenv(v)
		if val != "" {
			printf("  [SET]   %s = %s\n", v, val)
			addProxy(val)
		} else {
			printf("  [EMPTY] %s\n", v)
		}
	}
	printf("\n")
}

// ─── 2. System Proxy (Windows registry / PAC) ───────────────

func checkSystemProxy() {
	section("System Proxy Detection")

	if runtime.GOOS == "windows" {
		checkWindowsProxy()
	} else if runtime.GOOS == "darwin" {
		checkMacProxy()
	} else {
		println("  [SKIP] System proxy detection not supported on this OS")
		println("         Check /etc/environment or desktop settings manually")
	}

	// Detect Zscaler process
	if runtime.GOOS == "windows" {
		checkZscalerProcess()
	}

	printf("\n")
}

func checkWindowsProxy() {
	regBase := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

	// AutoConfigURL (PAC)
	subsection("Windows Proxy Settings (Registry)")
	pacURL := regQuery(regBase, "AutoConfigURL")
	if pacURL != "" {
		printf("  [SET]   AutoConfigURL = %s\n", pacURL)
		if strings.Contains(strings.ToLower(pacURL), "zscloud") || strings.Contains(strings.ToLower(pacURL), "zscaler") {
			println("  [!]     ** Zscaler PAC detected **")
			zscalerDetected = true
		}
		fetchPACFile(pacURL)
	} else {
		println("  [EMPTY] AutoConfigURL (no PAC file configured)")
	}

	// Manual proxy
	proxyEnable := regQuery(regBase, "ProxyEnable")
	proxyServer := regQuery(regBase, "ProxyServer")
	if proxyEnable == "0x1" || proxyEnable == "1" {
		printf("  [SET]   ProxyEnable = %s\n", proxyEnable)
		if proxyServer != "" {
			printf("  [SET]   ProxyServer = %s\n", proxyServer)
			addProxy(proxyServer)
		}
	} else {
		printf("  [OFF]   ProxyEnable = %s\n", proxyEnable)
		if proxyServer != "" {
			printf("  [SET]   ProxyServer = %s (but disabled)\n", proxyServer)
		}
	}
}

func checkMacProxy() {
	subsection("macOS Proxy Settings")
	out, err := runCmd(5*time.Second, "networksetup", "-getautoproxyurl", "Wi-Fi")
	if err == nil {
		printf("  Auto Proxy: %s\n", strings.TrimSpace(out))
		// Extract URL if enabled
		if strings.Contains(out, "Enabled: Yes") {
			re := regexp.MustCompile(`URL:\s*(\S+)`)
			if m := re.FindStringSubmatch(out); len(m) > 1 {
				fetchPACFile(m[1])
			}
		}
	}
	out, err = runCmd(5*time.Second, "networksetup", "-getwebproxy", "Wi-Fi")
	if err == nil {
		printf("  Web Proxy:  %s\n", strings.TrimSpace(out))
	}
}

func checkZscalerProcess() {
	subsection("Zscaler Process Detection")
	procs := []string{"ZSATunnel.exe", "ZscalerService.exe", "ZSAService.exe"}
	found := false
	for _, p := range procs {
		out, err := runCmd(5*time.Second, "tasklist", "/fi", fmt.Sprintf("imagename eq %s", p), "/nh")
		if err == nil && !strings.Contains(out, "No tasks") && strings.Contains(strings.ToLower(out), strings.ToLower(strings.TrimSuffix(p, ".exe"))) {
			printf("  [FOUND] %s is running\n", p)
			zscalerDetected = true
			found = true
		}
	}
	if !found {
		println("  [NONE]  No Zscaler processes detected")
	}
}

func regQuery(keyPath, valueName string) string {
	out, err := runCmd(5*time.Second, "reg", "query", keyPath, "/v", valueName)
	if err != nil {
		return ""
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, valueName) {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return parts[len(parts)-1]
			}
		}
	}
	return ""
}

func fetchPACFile(pacURL string) {
	subsection("PAC File Analysis")
	printf("  Fetching: %s\n", pacURL)

	// Try fetching PAC file (with InsecureSkipVerify since we might be behind Zscaler)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	resp, err := client.Get(pacURL)
	if err != nil {
		printf("  [FAIL] Cannot download PAC: %v\n", err)
		printf("         Try manually: curl -k \"%s\"\n", pacURL)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	pacContent := string(body)
	printf("  [OK]   PAC file downloaded (%d bytes)\n", len(body))

	// Extract PROXY directives
	re := regexp.MustCompile(`(?i)PROXY\s+([^\s;"']+:\d+)`)
	matches := re.FindAllStringSubmatch(pacContent, -1)
	seen := map[string]bool{}
	for _, m := range matches {
		proxy := m[1]
		if !seen[proxy] {
			seen[proxy] = true
			printf("  [PROXY] Found: %s\n", proxy)
			addProxy("http://" + proxy)
		}
	}

	if len(seen) == 0 {
		println("  [WARN]  No PROXY directives found in PAC file")
		// Show a snippet for manual inspection
		snippet := truncate(pacContent, 500)
		printf("  PAC snippet: %s\n", snippet)
	}
}

// ─── 3. DNS Resolution ──────────────────────────────────────

func checkDNS() {
	section("DNS Resolution")

	hosts := []string{
		"github.com",
		"api.github.com",
		"raw.githubusercontent.com",
		"ssh.github.com",
	}

	allOK := true
	for _, host := range hosts {
		start := time.Now()
		addrs, err := net.LookupHost(host)
		elapsed := time.Since(start)

		if err != nil {
			printf("  [FAIL] %s -- %v (took %v)\n", host, err, elapsed)
			allOK = false
		} else {
			printf("  [OK]   %s -> %s (took %v)\n", host, strings.Join(addrs, ", "), elapsed)
		}
	}
	dnsOK = allOK
	printf("\n")
}

// ─── 4. TCP Connectivity ────────────────────────────────────

func checkTCPConnect() {
	section("TCP Connectivity")

	targets := []struct {
		host string
		port string
		desc string
		flag *bool
	}{
		{"github.com", "443", "HTTPS", &tcpHTTPSOK},
		{"github.com", "22", "SSH", &sshPort22OK},
		{"github.com", "9418", "Git Protocol", nil},
		{"ssh.github.com", "443", "SSH over HTTPS port", &sshPort443OK},
	}

	for _, t := range targets {
		addr := net.JoinHostPort(t.host, t.port)
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		elapsed := time.Since(start)

		if err != nil {
			printf("  [FAIL] %s (%s) -- %v (took %v)\n", addr, t.desc, err, elapsed)
		} else {
			conn.Close()
			printf("  [OK]   %s (%s) -- connected (took %v)\n", addr, t.desc, elapsed)
			if t.flag != nil {
				*t.flag = true
			}
		}
	}
	printf("\n")
}

// ─── 5. HTTPS Direct ────────────────────────────────────────

func checkHTTPSDirect() {
	section("HTTPS Direct Connectivity")

	testURL := "https://api.github.com"

	subsection("Standard TLS")
	ok := testHTTPGet(testURL, &http.Client{Timeout: 15 * time.Second})
	if ok {
		httpsDirectOK = true
	}

	if !ok {
		subsection("Skip TLS Verify (check if cert issue)")
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		ok2 := testHTTPGet(testURL, &http.Client{Transport: transport, Timeout: 15 * time.Second})
		if ok2 {
			println("  [!]     Direct HTTPS works with InsecureSkipVerify")
			println("          -> Certificate trust issue (likely TLS interception)")
			httpsDirectOK = false // only works insecure, not usable for git
		}
	}

	printf("\n")
}

// ─── 6. HTTPS Via Proxy ─────────────────────────────────────

func checkHTTPSViaProxy() {
	section("HTTPS Via Proxy")

	if len(foundProxies) == 0 {
		// Also try common local proxy ports
		localPorts := []string{"8080", "3128", "9480", "9000", "8443", "10080"}
		for _, port := range localPorts {
			addr := net.JoinHostPort("127.0.0.1", port)
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err == nil {
				conn.Close()
				printf("  [FOUND] Local proxy at %s\n", addr)
				addProxy(fmt.Sprintf("http://127.0.0.1:%s", port))
			}
		}
	}

	if len(foundProxies) == 0 {
		println("  [SKIP] No proxies discovered to test")
		printf("\n")
		return
	}

	testURL := "https://api.github.com"
	for _, p := range foundProxies {
		proxyURL, err := url.Parse(p)
		if err != nil {
			printf("  [SKIP] Invalid proxy URL: %s\n", p)
			continue
		}

		printf("\n  -- Testing proxy: %s --\n", p)

		// Test with TLS verification
		transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
		ok := testHTTPGet(testURL, client)

		if ok {
			httpsViaProxyOK = true
			workingProxy = p
			printf("  [OK]   Proxy %s works for GitHub HTTPS!\n", p)
			break
		}

		// Test without TLS verification
		transport2 := &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client2 := &http.Client{Transport: transport2, Timeout: 15 * time.Second}
		ok2 := testHTTPGet(testURL, client2)

		if ok2 {
			httpsViaProxyOK = true
			workingProxy = p
			needCustomCert = true
			printf("  [OK]   Proxy %s works (needs custom CA cert for TLS)\n", p)
			break
		}
	}

	if !httpsViaProxyOK && len(foundProxies) > 0 {
		println("  [FAIL] None of the discovered proxies could reach GitHub")
	}

	printf("\n")
}

// ─── 7. TLS Certificate ─────────────────────────────────────

func checkTLSCert() {
	section("TLS Certificate Chain (github.com:443)")

	// Try direct TLS
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp", "github.com:443",
		&tls.Config{InsecureSkipVerify: false},
	)
	if err != nil {
		printf("  [FAIL] TLS handshake failed: %v\n", err)

		// Retry with InsecureSkipVerify
		subsection("Retry with InsecureSkipVerify")
		conn2, err2 := tls.DialWithDialer(
			&net.Dialer{Timeout: 10 * time.Second},
			"tcp", "github.com:443",
			&tls.Config{InsecureSkipVerify: true},
		)
		if err2 != nil {
			printf("  [FAIL] Even insecure TLS failed: %v\n", err2)
			println("         Connection is blocked at network level (DPI/firewall)")
		} else {
			printCertChain(conn2)
			conn2.Close()
		}

		// If we have a working proxy, try TLS through proxy to inspect cert
		if workingProxy != "" {
			subsection("TLS via proxy (inspect cert chain)")
			proxyURL, _ := url.Parse(workingProxy)
			transport := &http.Transport{
				Proxy:           http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
			resp, err := client.Get("https://api.github.com")
			if err == nil {
				if resp.TLS != nil {
					for i, cert := range resp.TLS.PeerCertificates {
						printf("  Certificate #%d:\n", i)
						printf("    Subject: %s\n", cert.Subject)
						printf("    Issuer:  %s\n", cert.Issuer)
						issuerStr := cert.Issuer.String()
						if strings.Contains(strings.ToLower(issuerStr), "zscaler") {
							println("    [!] ZSCALER TLS INTERCEPTION DETECTED")
							zscalerDetected = true
							needCustomCert = true
						}
					}
				}
				resp.Body.Close()
			}
		}

		printf("\n")
		return
	}
	defer conn.Close()

	tlsDirectOK = true
	printCertChain(conn)
	printf("\n")
}

func printCertChain(conn *tls.Conn) {
	state := conn.ConnectionState()
	printf("  TLS Version: %s\n", tlsVersionName(state.Version))
	printf("  Cipher Suite: %s\n", tls.CipherSuiteName(state.CipherSuite))
	printf("  Server Name: %s\n", state.ServerName)
	printf("\n")

	for i, cert := range state.PeerCertificates {
		printf("  Certificate #%d:\n", i)
		printf("    Subject:  %s\n", cert.Subject)
		printf("    Issuer:   %s\n", cert.Issuer)
		printf("    NotBefore: %s\n", cert.NotBefore.Format(time.RFC3339))
		printf("    NotAfter:  %s\n", cert.NotAfter.Format(time.RFC3339))
		if len(cert.DNSNames) > 0 {
			printf("    DNS Names: %s\n", strings.Join(cert.DNSNames, ", "))
		}
		issuerStr := cert.Issuer.String()
		if strings.Contains(strings.ToLower(issuerStr), "zscaler") {
			println("    [!] ZSCALER TLS INTERCEPTION DETECTED")
			zscalerDetected = true
		}
	}
}

// ─── 8. Git Setup ───────────────────────────────────────────

func checkGitSetup() {
	section("Git Setup")

	// Git version
	subsection("Git Version")
	out, err := runCmd(5*time.Second, "git", "--version")
	if err != nil {
		println("  [FAIL] git is not installed or not in PATH")
		return
	}
	gitInstalled = true
	printf("  [OK]   %s\n", strings.TrimSpace(out))

	// Git config (proxy/SSL related)
	subsection("Git Config (proxy/SSL)")
	configs := []string{
		"http.proxy", "https.proxy",
		"http.sslVerify", "http.sslCAInfo", "http.sslCAPath",
		"http.sslBackend", "url.https://github.com/.insteadOf",
	}
	for _, key := range configs {
		val, err := runCmd(5*time.Second, "git", "config", "--global", "--get", key)
		val = strings.TrimSpace(val)
		if err != nil || val == "" {
			printf("  [EMPTY] git config --global %s\n", key)
		} else {
			printf("  [SET]   git config --global %s = %s\n", key, val)
		}
	}

	// SSH config
	subsection("SSH Config")
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}

	sshConfigPath := filepath.Join(home, ".ssh", "config")
	if data, err := os.ReadFile(sshConfigPath); err == nil {
		printf("  [OK]   %s exists (%d bytes)\n", sshConfigPath, len(data))
		content := string(data)
		// Check for github.com entry
		if strings.Contains(strings.ToLower(content), "github.com") {
			println("  [OK]   Contains github.com configuration")
			if strings.Contains(content, "ssh.github.com") || strings.Contains(content, "443") {
				println("  [OK]   SSH over 443 appears configured")
				sshConfigOK = true
			}
		} else {
			println("  [WARN] No github.com entry found")
		}
	} else {
		printf("  [WARN] %s not found\n", sshConfigPath)
	}

	// SSH keys
	subsection("SSH Keys")
	keyTypes := []struct {
		name string
		file string
	}{
		{"Ed25519", "id_ed25519"},
		{"RSA", "id_rsa"},
		{"ECDSA", "id_ecdsa"},
	}
	for _, kt := range keyTypes {
		pubPath := filepath.Join(home, ".ssh", kt.file+".pub")
		if _, err := os.Stat(pubPath); err == nil {
			printf("  [OK]   %s key found: %s\n", kt.name, pubPath)
			sshKeyFound = true
			sshKeyType = kt.name
		}
	}
	if !sshKeyFound {
		println("  [WARN] No SSH keys found in ~/.ssh/")
	}

	// SSH agent
	out, err = runCmd(5*time.Second, "ssh-add", "-l")
	if err == nil && !strings.Contains(out, "no identities") {
		printf("  [OK]   SSH agent has keys loaded:\n")
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line != "" {
				printf("         %s\n", line)
			}
		}
	} else {
		println("  [WARN] SSH agent has no keys loaded")
	}

	printf("\n")
}

// ─── 9. Git Operations ──────────────────────────────────────

func checkGitOperations() {
	section("Git Operations Test")

	if !gitInstalled {
		println("  [SKIP] git is not installed")
		printf("\n")
		return
	}

	testRepo := "https://github.com/dw101-cn/network-diag.git"
	testRepoSSH := "git@github.com:dw101-cn/network-diag.git"

	// HTTPS direct
	subsection("git ls-remote (HTTPS direct)")
	out, err := runCmd(20*time.Second, "git", "ls-remote", "--heads", testRepo)
	if err != nil {
		printf("  [FAIL] %v\n", truncate(strings.TrimSpace(out+err.Error()), 200))
	} else {
		println("  [OK]   HTTPS direct works!")
		gitHTTPSDirectOK = true
		printLsRemoteResult(out)
	}

	// HTTPS via proxy
	if !gitHTTPSDirectOK && workingProxy != "" {
		subsection(fmt.Sprintf("git ls-remote (HTTPS via proxy %s)", workingProxy))

		// With TLS verify
		out, err := runCmd(20*time.Second, "git",
			"-c", "http.proxy="+workingProxy,
			"ls-remote", "--heads", testRepo)
		if err != nil {
			printf("  [FAIL] With TLS verify: %v\n", truncate(strings.TrimSpace(out+err.Error()), 200))

			// Without TLS verify
			out, err = runCmd(20*time.Second, "git",
				"-c", "http.proxy="+workingProxy,
				"-c", "http.sslVerify=false",
				"ls-remote", "--heads", testRepo)
			if err != nil {
				printf("  [FAIL] Without TLS verify: %v\n", truncate(strings.TrimSpace(out+err.Error()), 200))
			} else {
				println("  [OK]   HTTPS via proxy works (TLS verify disabled)")
				gitHTTPSProxyOK = true
				needCustomCert = true
				printLsRemoteResult(out)
			}
		} else {
			println("  [OK]   HTTPS via proxy works!")
			gitHTTPSProxyOK = true
			printLsRemoteResult(out)
		}
	}

	// SSH direct (port 22)
	subsection("git ls-remote (SSH port 22)")
	out, err = runCmd(15*time.Second, "git", "ls-remote", "--heads", testRepoSSH)
	if err != nil {
		errMsg := strings.TrimSpace(out)
		if errMsg == "" {
			errMsg = err.Error()
		}
		printf("  [FAIL] %s\n", truncate(errMsg, 200))
	} else {
		println("  [OK]   SSH direct works!")
		gitSSHDirectOK = true
		printLsRemoteResult(out)
	}

	// SSH over 443 (if direct SSH failed and SSH config exists)
	if !gitSSHDirectOK && sshPort443OK {
		subsection("git ls-remote (SSH over 443)")
		// Use GIT_SSH_COMMAND to force port 443
		env := append(os.Environ(), `GIT_SSH_COMMAND=ssh -o "HostName=ssh.github.com" -o "Port=443"`)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", testRepoSSH)
		cmd.Env = env
		outBytes, err := cmd.CombinedOutput()
		out = string(outBytes)
		if err != nil {
			errMsg := strings.TrimSpace(out)
			if errMsg == "" {
				errMsg = err.Error()
			}
			printf("  [FAIL] %s\n", truncate(errMsg, 200))
		} else {
			println("  [OK]   SSH over 443 works!")
			gitSSH443OK = true
			printLsRemoteResult(out)
		}
	}

	printf("\n")
}

func printLsRemoteResult(out string) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if line != "" {
			printf("         %s\n", truncate(line, 100))
		}
	}
}

// ─── 10. SSH Protocol ───────────────────────────────────────

func checkSSHProtocol() {
	section("SSH Protocol Tests")

	// SSH to github.com:22
	subsection("SSH banner github.com:22")
	if sshPort22OK {
		banner := readSSHBanner("github.com:22")
		if banner != "" {
			printf("  [OK]   %s\n", banner)
		}
	} else {
		println("  [FAIL] Port 22 is blocked (TCP timeout)")
	}

	// SSH to ssh.github.com:443
	subsection("SSH banner ssh.github.com:443")
	if sshPort443OK {
		banner := readSSHBanner("ssh.github.com:443")
		if banner != "" {
			printf("  [OK]   %s\n", banner)
		} else {
			println("  [WARN] TCP connected but no SSH banner (may be intercepted)")
		}
	} else {
		println("  [FAIL] ssh.github.com:443 is not reachable")
	}

	printf("\n")
}

func readSSHBanner(addr string) string {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(buf[:n]))
}

// ─── 11. Summary ────────────────────────────────────────────

func printSummary() {
	printf("\n")
	println("========================================")
	println("  DIAGNOSIS SUMMARY")
	println("========================================")

	// Status table
	printf("\n")
	printf("  %-30s %s\n", "DNS Resolution:", boolStatus(dnsOK))
	printf("  %-30s %s\n", "TCP to github.com:443:", boolStatus(tcpHTTPSOK))
	printf("  %-30s %s\n", "TCP to github.com:22:", boolStatus(sshPort22OK))
	printf("  %-30s %s\n", "TCP to ssh.github.com:443:", boolStatus(sshPort443OK))
	printf("  %-30s %s\n", "HTTPS Direct:", boolStatus(httpsDirectOK))
	printf("  %-30s %s\n", "HTTPS Via Proxy:", boolStatus(httpsViaProxyOK))
	printf("  %-30s %s\n", "TLS Direct:", boolStatus(tlsDirectOK))
	printf("  %-30s %s\n", "Zscaler Detected:", boolYesNo(zscalerDetected))
	if gitInstalled {
		printf("  %-30s %s\n", "Git HTTPS Direct:", boolStatus(gitHTTPSDirectOK))
		printf("  %-30s %s\n", "Git HTTPS Via Proxy:", boolStatus(gitHTTPSProxyOK))
		printf("  %-30s %s\n", "Git SSH (port 22):", boolStatus(gitSSHDirectOK))
		printf("  %-30s %s\n", "Git SSH (port 443):", boolStatus(gitSSH443OK))
	} else {
		printf("  %-30s %s\n", "Git:", "NOT INSTALLED")
	}

	// Recommendations
	printf("\n")
	println("========================================")
	println("  RECOMMENDED FIX")
	println("========================================")
	printf("\n")

	if httpsDirectOK || gitHTTPSDirectOK {
		println("  Everything works! No fix needed.")
		println("  Use HTTPS to clone:")
		println("    git clone https://github.com/dw101-cn/network-diag.git")
		return
	}

	if gitSSHDirectOK {
		println("  SSH works! Use SSH to clone:")
		println("    git clone git@github.com:dw101-cn/network-diag.git")
		return
	}

	step := 1

	// Case: proxy works for HTTPS
	if httpsViaProxyOK && workingProxy != "" {
		printf("  Step %d: Configure Git to use proxy\n", step)
		printf("    git config --global http.proxy %s\n", workingProxy)
		printf("\n")
		step++

		if needCustomCert {
			printf("  Step %d: Export Zscaler root CA certificate\n", step)
			println("    Option A - Export from Windows cert store:")
			println("      1. Win+R -> certmgr.msc")
			println("      2. Trusted Root Certification Authorities -> Certificates")
			println("      3. Find 'Zscaler' -> Right click -> All Tasks -> Export")
			println("      4. Choose 'Base-64 encoded X.509 (.CER)'")
			printf("      5. Save as: C:\\Users\\%%USERNAME%%\\ZscalerRootCA.pem\n")
			printf("\n")
			println("    Option B - Export via PowerShell:")
			println("      $cert = Get-ChildItem Cert:\\LocalMachine\\Root | Where-Object {$_.Subject -match 'Zscaler'} | Select-Object -First 1")
			printf("      [System.IO.File]::WriteAllText(\"$env:USERPROFILE\\ZscalerRootCA.pem\", \"-----BEGIN CERTIFICATE-----`n$([Convert]::ToBase64String($cert.RawData, 'InsertLineBreaks'))`n-----END CERTIFICATE-----\")\n")
			printf("\n")
			step++

			printf("  Step %d: Configure Git to trust the certificate\n", step)
			printf("    git config --global http.sslCAInfo \"%%USERPROFILE%%\\ZscalerRootCA.pem\"\n")
			printf("\n")
			step++
		}

		printf("  Step %d: Clone via HTTPS\n", step)
		println("    git clone https://github.com/dw101-cn/network-diag.git")
		printf("\n")
		return
	}

	// Case: SSH over 443 works
	if gitSSH443OK || sshPort443OK {
		printf("  Step %d: Configure SSH to use port 443\n", step)
		println("    Add to ~/.ssh/config:")
		println("      Host github.com")
		println("        Hostname ssh.github.com")
		println("        Port 443")
		println("        User git")
		printf("\n")
		step++

		if !sshKeyFound {
			printf("  Step %d: Generate SSH key\n", step)
			println("    ssh-keygen -t ed25519 -C \"your_email@example.com\"")
			println("    Then add the public key to GitHub: Settings -> SSH Keys")
			printf("\n")
			step++
		}

		printf("  Step %d: Clone via SSH\n", step)
		println("    git clone git@github.com:dw101-cn/network-diag.git")
		printf("\n")
		return
	}

	// Nothing works
	println("  [!] No working connection method found.")
	println("")
	println("  Possible actions:")
	println("    1. Ask IT to whitelist github.com in the firewall/proxy")
	println("    2. Ask IT for the correct proxy address for CLI tools")
	println("    3. If Zscaler is in use, ask IT for the Zscaler proxy address")
	printf("       (PAC file may not contain usable proxy for CLI)\n")
	printf("\n")
}

// ─── Helpers ─────────────────────────────────────────────────

func testHTTPGet(targetURL string, client *http.Client) bool {
	start := time.Now()
	resp, err := client.Get(targetURL)
	elapsed := time.Since(start)

	if err != nil {
		printf("  [FAIL] GET %s\n", targetURL)
		printf("         Error: %v (took %v)\n", err, elapsed)
		return false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	printf("  [OK]   GET %s\n", targetURL)
	printf("         Status: %d, Body: %s (took %v)\n", resp.StatusCode, truncate(string(body), 150), elapsed)
	return resp.StatusCode < 500
}

func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func addProxy(proxyURL string) {
	// Normalize
	p := strings.TrimSpace(proxyURL)
	if p == "" {
		return
	}
	if !strings.Contains(p, "://") {
		p = "http://" + p
	}
	for _, existing := range foundProxies {
		if existing == p {
			return
		}
	}
	foundProxies = append(foundProxies, p)
}

func boolStatus(b bool) string {
	if b {
		return "[OK]"
	}
	return "[FAIL]"
}

func boolYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

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

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
