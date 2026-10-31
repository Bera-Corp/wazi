// Command recordtool canonicalizes, hashes and verifies offline pilot records.
// Private signing keys are never read by this command.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Bera-Corp/wazi/services/internal/agentclient"
	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: recordtool canonicalize|digest|verify INPUT [OUTPUT|POLICY]")
	}
	raw, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return err
	}
	switch args[0] {
	case "render-package":
		if len(args) != 4 {
			return fmt.Errorf("render-package requires manifest, protected instance config and output")
		}
		configRaw, err := os.ReadFile(args[2])
		if err != nil {
			return err
		}
		if _, err := verify.ParseJSON(configRaw); err != nil {
			return err
		}
		var config agentclient.PackageInstanceConfig
		if err := json.Unmarshal(configRaw, &config); err != nil {
			return err
		}
		compose, err := agentclient.RenderReviewedPackageCompose(raw, config)
		if err != nil {
			return err
		}
		return os.WriteFile(args[3], compose, 0600)
	case "render-pilot":
		if len(args) != 4 {
			return fmt.Errorf("render-pilot requires manifest, image-config map and output")
		}
		mappingRaw, err := os.ReadFile(args[2])
		if err != nil {
			return err
		}
		var mapping map[string]string
		if err := json.Unmarshal(mappingRaw, &mapping); err != nil {
			return err
		}
		compose, err := agentclient.RenderPilotCompose(raw, mapping)
		if err != nil {
			return err
		}
		return os.WriteFile(args[3], compose, 0600)
	case "record-digest":
		digest, err := verify.CanonicalDigest(value)
		if err != nil {
			return err
		}
		fmt.Print(digest)
		return nil
	case "canonicalize":
		if len(args) != 3 {
			return fmt.Errorf("canonicalize requires output path")
		}
		canonical, err := verify.CanonicalJSON(value)
		if err != nil {
			return err
		}
		return os.WriteFile(args[2], canonical, 0600)
	case "digest":
		digest, err := verify.PayloadDigest(raw)
		if err != nil {
			return err
		}
		fmt.Print(digest)
		return nil
	case "verify":
		if len(args) != 3 {
			return fmt.Errorf("verify requires an operator policy")
		}
		policy, err := verify.LoadPolicyFile(args[2])
		if err != nil {
			return err
		}
		record, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("record must be an object")
		}
		switch {
		case record["appId"] != nil:
			err = contracts.ValidateAppManifest(raw)
		case record["procedureId"] != nil:
			err = contracts.ValidateProcedure(raw)
		case record["listId"] != nil:
			err = contracts.ValidateRevocationRecord(raw)
		default:
			return fmt.Errorf("unsupported signed record type")
		}
		if err != nil {
			return err
		}
		verifyRecord := verify.VerifyRecord
		if record["listId"] != nil {
			verifyRecord = verify.VerifySignedRecord
		}
		if err := verifyRecord(raw, policy); err != nil {
			return err
		}
		fmt.Println("schema and signature verified")
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
