// backup-keygen creates protected age identity files without printing key bytes.
package main

import (
	"filippo.io/age"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func writeNew(path string, raw []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err = file.Write(raw); err != nil {
		return err
	}
	return file.Sync()
}

func main() {
	identityFile := flag.String("identity-file", "", "protected private identity destination")
	recipientFile := flag.String("recipient-file", "", "public recipient destination")
	flag.Parse()
	if *identityFile == "" || *recipientFile == "" || filepath.Clean(*identityFile) == filepath.Clean(*recipientFile) {
		fmt.Fprintln(os.Stderr, "distinct identity and recipient paths required")
		os.Exit(2)
	}
	identity, err := age.GenerateX25519Identity()
	if err == nil {
		err = writeNew(*identityFile, []byte(identity.String()+"\n"))
	}
	if err == nil {
		err = writeNew(*recipientFile, []byte(identity.Recipient().String()+"\n"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "key generation failed; inspect destination files before retrying")
		os.Exit(1)
	}
	fmt.Println("Protected identity and public recipient created")
}
