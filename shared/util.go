package shared

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	lxd "github.com/lxc/lxd/shared"
	"golang.org/x/sys/unix"
	"gopkg.in/flosch/pongo2.v3"
	yaml "gopkg.in/yaml.v2"
)

// EnvVariable represents a environment variable.
type EnvVariable struct {
	Value string
	Set   bool
}

// Environment represents a set of environment variables.
type Environment map[string]EnvVariable

// Copy copies a file.
func Copy(src, dest string) error {
	var err error

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("Failed to open file %q: %w", src, err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("Failed to create file %q: %w", dest, err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("Failed to copy file: %w", err)
	}

	return destFile.Sync()
}

// RunCommand runs a command hereby setting the SHELL and PATH env variables,
// and redirecting the process's stdout and stderr to the real stdout and stderr
// respectively.
func RunCommand(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// RunScript runs a script hereby setting the SHELL and PATH env variables,
// and redirecting the process's stdout and stderr to the real stdout and stderr
// respectively.
func RunScript(content string) error {
	fd, err := unix.MemfdCreate("tmp", 0)
	if err != nil {
		return fmt.Errorf("Failed to create memfd: %w", err)
	}
	defer unix.Close(fd)

	_, err = unix.Write(int(fd), []byte(content))
	if err != nil {
		return fmt.Errorf("Failed to write to memfd: %w", err)
	}

	fdPath := fmt.Sprintf("/proc/self/fd/%d", fd)

	return RunCommand(fdPath)
}

// GetSignedContent verifies the provided file, and returns its decrypted (plain) content.
func GetSignedContent(signedFile string, keys []string, keyserver string) ([]byte, error) {
	keyring, err := CreateGPGKeyring(keyserver, keys)
	if err != nil {
		return nil, err
	}

	gpgDir := path.Dir(keyring)
	defer os.RemoveAll(gpgDir)

	out, err := exec.Command("gpg", "--homedir", gpgDir, "--keyring", keyring,
		"--decrypt", signedFile).Output()
	if err != nil {
		return nil, fmt.Errorf("Failed to get file content: %s: %w", out, err)
	}

	return out, nil
}

// VerifyFile verifies a file using gpg.
func VerifyFile(signedFile, signatureFile string, keys []string, keyserver string) (bool, error) {
	keyring, err := CreateGPGKeyring(keyserver, keys)
	if err != nil {
		return false, err
	}
	gpgDir := path.Dir(keyring)
	defer os.RemoveAll(gpgDir)

	if signatureFile != "" {
		out, err := lxd.RunCommand("gpg", "--homedir", gpgDir, "--keyring", keyring,
			"--verify", signatureFile, signedFile)
		if err != nil {
			return false, fmt.Errorf("Failed to verify: %s: %w", out, err)
		}
	} else {
		out, err := lxd.RunCommand("gpg", "--homedir", gpgDir, "--keyring", keyring,
			"--verify", signedFile)
		if err != nil {
			return false, fmt.Errorf("Failed to verify: %s: %w", out, err)
		}
	}

	return true, nil
}

func recvGPGKeys(gpgDir string, keyserver string, keys []string) (bool, error) {
	args := []string{"--homedir", gpgDir}

	var fingerprints []string
	var publicKeys []string

	for _, k := range keys {
		if strings.HasPrefix(strings.TrimSpace(k), "-----BEGIN PGP PUBLIC KEY BLOCK-----") {
			publicKeys = append(publicKeys, strings.TrimSpace(k))
		} else {
			fingerprints = append(fingerprints, strings.TrimSpace(k))
		}
	}

	for _, f := range publicKeys {
		args := append(args, "--import")

		cmd := exec.Command("gpg", args...)
		cmd.Stdin = strings.NewReader(f)
		cmd.Env = append(os.Environ(), "LANG=C.UTF-8")

		var buffer bytes.Buffer
		cmd.Stderr = &buffer

		err := cmd.Run()
		if err != nil {
			return false, fmt.Errorf("Failed to run: %s: %s", strings.Join(cmd.Args, " "), strings.TrimSpace(buffer.String()))
		}
	}

	if keyserver != "" {
		args = append(args, "--keyserver", keyserver)
	}

	args = append(args, append([]string{"--recv-keys"}, fingerprints...)...)

	_, out, err := lxd.RunCommandSplit(append(os.Environ(), "LANG=C.UTF-8"), nil, "gpg", args...)
	if err != nil {
		return false, err
	}

	// Verify output
	var importedKeys []string
	var missingKeys []string
	lines := strings.Split(out, "\n")

	for _, l := range lines {
		if strings.HasPrefix(l, "gpg: key ") && (strings.HasSuffix(l, " imported") || strings.HasSuffix(l, " not changed")) {
			key := strings.Split(l, " ")
			importedKeys = append(importedKeys, strings.Split(key[2], ":")[0])
		}
	}

	// Figure out which key(s) couldn't be imported
	if len(importedKeys) < len(fingerprints) {
		for _, j := range fingerprints {
			found := false

			for _, k := range importedKeys {
				if strings.HasSuffix(j, k) {
					found = true
				}
			}

			if !found {
				missingKeys = append(missingKeys, j)
			}
		}

		return false, fmt.Errorf("Failed to import keys: %s", strings.Join(missingKeys, " "))
	}

	return true, nil
}

// CreateGPGKeyring creates a new GPG keyring.
func CreateGPGKeyring(keyserver string, keys []string) (string, error) {
	gpgDir, err := ioutil.TempDir(os.TempDir(), "distrobuilder.")
	if err != nil {
		return "", fmt.Errorf("Failed to create gpg directory: %w", err)
	}

	err = os.MkdirAll(gpgDir, 0700)
	if err != nil {
		return "", err
	}

	var ok bool

	for i := 0; i < 3; i++ {
		ok, err = recvGPGKeys(gpgDir, keyserver, keys)
		if ok {
			break
		}

		time.Sleep(2 * time.Second)
	}

	if !ok {
		return "", err
	}

	// Export keys to support gpg1 and gpg2
	out, err := lxd.RunCommand("gpg", "--homedir", gpgDir, "--export", "--output",
		filepath.Join(gpgDir, "distrobuilder.gpg"))
	if err != nil {
		os.RemoveAll(gpgDir)
		return "", fmt.Errorf("Failed to export keyring: %s: %w", out, err)
	}

	return filepath.Join(gpgDir, "distrobuilder.gpg"), nil
}

// Pack creates an uncompressed tarball.
func Pack(filename, compression, path string, args ...string) (string, error) {
	err := RunCommand("tar", append([]string{"--xattrs", "-cf", filename, "-C", path}, args...)...)
	if err != nil {
		// Clean up incomplete tarball
		os.Remove(filename)
		return "", fmt.Errorf("Failed to create tarball: %w", err)
	}

	return compressTarball(filename, compression)
}

// PackUpdate updates an existing tarball.
func PackUpdate(filename, compression, path string, args ...string) (string, error) {
	err := RunCommand("tar", append([]string{"--xattrs", "-uf", filename, "-C", path}, args...)...)
	if err != nil {
		return "", fmt.Errorf("Failed to update tarball: %w", err)
	}

	return compressTarball(filename, compression)
}

// compressTarball compresses a tarball, or not.
func compressTarball(filename, compression string) (string, error) {
	fileExtension := ""

	switch compression {
	case "lzop", "zstd":
		// Remove the uncompressed file as the compress fails to do so.
		defer os.Remove(filename)
		fallthrough
	case "bzip2", "xz", "lzip", "lzma", "gzip":
		err := RunCommand(compression, "-f", filename)
		if err != nil {
			return "", fmt.Errorf("Failed to compress tarball %q: %w", filename, err)
		}
	}

	switch compression {
	case "lzop":
		fileExtension = "lzo"
	case "zstd":
		fileExtension = "zst"
	case "bzip2":
		fileExtension = "bz2"
	case "xz":
		fileExtension = "xz"
	case "lzip":
		fileExtension = "lz"
	case "lzma":
		fileExtension = "lzma"
	case "gzip":
		fileExtension = "gz"
	}

	if fileExtension == "" {
		return filename, nil
	}

	return fmt.Sprintf("%s.%s", filename, fileExtension), nil
}

//GetExpiryDate returns an expiry date based on the creationDate and format.
func GetExpiryDate(creationDate time.Time, format string) time.Time {
	regex := regexp.MustCompile(`(?:(\d+)(s|m|h|d|w))*`)
	expiryDate := creationDate

	for _, match := range regex.FindAllStringSubmatch(format, -1) {
		// Ignore empty matches
		if match[0] == "" {
			continue
		}

		var duration time.Duration

		switch match[2] {
		case "s":
			duration = time.Second
		case "m":
			duration = time.Minute
		case "h":
			duration = time.Hour
		case "d":
			duration = 24 * time.Hour
		case "w":
			duration = 7 * 24 * time.Hour
		}

		// Ignore any error since it will be an integer.
		value, _ := strconv.Atoi(match[1])
		expiryDate = expiryDate.Add(time.Duration(value) * duration)
	}

	return expiryDate
}

// RenderTemplate renders a pongo2 template.
func RenderTemplate(template string, iface interface{}) (string, error) {
	// Serialize interface
	data, err := yaml.Marshal(iface)
	if err != nil {
		return "", err
	}

	// Decode document and write it to a pongo2 Context
	var ctx pongo2.Context
	yaml.Unmarshal(data, &ctx)

	// Load template from string
	tpl, err := pongo2.FromString("{% autoescape off %}" + template + "{% endautoescape %}")
	if err != nil {
		return "", err
	}

	// Get rendered template
	ret, err := tpl.Execute(ctx)
	if err != nil {
		return ret, err
	}

	// Looks like we're nesting templates so run pongo again
	if strings.Contains(ret, "{{") || strings.Contains(ret, "{%") {
		return RenderTemplate(ret, iface)
	}

	return ret, err
}

// SetEnvVariables sets the provided environment variables and returns the
// old ones.
func SetEnvVariables(env Environment) Environment {
	oldEnv := Environment{}

	for k, v := range env {
		// Check whether the env variables are set at the moment
		oldVal, set := os.LookupEnv(k)

		// Store old env variables
		oldEnv[k] = EnvVariable{
			Value: oldVal,
			Set:   set,
		}

		if v.Set {
			os.Setenv(k, v.Value)
		} else {
			os.Unsetenv(k)
		}
	}

	return oldEnv
}

// GetTargetDir returns the path to which source files are downloaded.
func GetTargetDir(def DefinitionImage) string {
	targetDir := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s-%s", def.Distribution, def.Release, def.ArchitectureMapped))
	targetDir = strings.Replace(targetDir, " ", "", -1)
	targetDir = strings.ToLower(targetDir)

	return targetDir
}

func getChecksum(fname string, hashLen int, r io.Reader) []string {
	scanner := bufio.NewScanner(r)

	var matches []string
	var result []string

	for scanner.Scan() {
		if !strings.Contains(scanner.Text(), fname) {
			continue
		}

		for _, s := range strings.Split(scanner.Text(), " ") {
			m, _ := regexp.MatchString("[[:xdigit:]]+", s)
			if !m {
				continue
			}

			if hashLen == 0 || hashLen == len(strings.TrimSpace(s)) {
				matches = append(matches, scanner.Text())
			}
		}
	}

	// Check common checksum file (pattern: "<hash> <filename>") with the exact filename
	for _, m := range matches {
		fields := strings.Split(m, " ")

		if strings.TrimSpace(fields[len(fields)-1]) == fname {
			result = append(result, strings.TrimSpace(fields[0]))
		}
	}

	if len(result) > 0 {
		return result
	}

	// Check common checksum file (pattern: "<hash> <filename>") which contains the filename
	for _, m := range matches {
		fields := strings.Split(m, " ")

		if strings.Contains(strings.TrimSpace(fields[len(fields)-1]), fname) {
			result = append(result, strings.TrimSpace(fields[0]))
		}
	}

	if len(result) > 0 {
		return result
	}

	// Special case: CentOS
	for _, m := range matches {
		for _, s := range strings.Split(m, " ") {
			m, _ := regexp.MatchString("[[:xdigit:]]+", s)
			if !m {
				continue
			}

			if hashLen == 0 || hashLen == len(strings.TrimSpace(s)) {
				result = append(result, s)
			}
		}
	}

	if len(result) > 0 {
		return result
	}

	return nil
}

// RsyncLocal copies src to dest using rsync.
func RsyncLocal(src string, dest string) error {
	err := RunCommand("rsync", "-aHASX", "--devices", src, dest)
	if err != nil {
		return fmt.Errorf("Failed to copy %q to %q: %w", src, dest, err)
	}

	return nil
}

// Retry retries a function up to <attempts> times. This is especially useful for networking.
func Retry(f func() error, attempts uint) error {
	var err error

	for i := uint(0); i < attempts; i++ {
		err = f()
		if err == nil {
			break
		}

		time.Sleep(time.Second)
	}

	return err
}
