package oci

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/go-ini/ini"
	conf "github.com/oracle/oci-go-sdk/common"
)

func testConfig(accessConfFile *os.File) map[string]interface{} {
	return map[string]interface{}{
		"availability_domain": "aaaa:US-ASHBURN-AD-1",
		"access_cfg_file":     accessConfFile.Name(),

		"tenancy_ocid": "ocid1.tenancy.oc1..aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"user_ocid":    "ocid1.user.oc1..aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"region":       "us-ashburn-1",

		"fingerprint": "70:04:5z:b3:19:ab:90:75:a4:1f:50:d4:c7:c3:33:20",

		// Image
		"base_image_ocid": "ocid1.image.oc1.iad.aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"shape":           "VM.Standard1.2",
		"image_name":      "HelloWorld",

		// Networking
		"subnet_ocid": "ocid1.subnet.oc1.iad.aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",

		// Comm
		"ssh_username":   "ubuntu",
		"use_private_ip": false,
	}
}

func getField(c *conf.ConfigurationProvider, field string) string {
	r := reflect.ValueOf(c)
	f := reflect.Indirect(r).FieldByName(field)
	return string(f.String()) //TODO(harveylowndes) Wrap with string?
}

func TestConfig(t *testing.T) {
	// Shared set-up and defered deletion

	cfg, keyFile, err := baseTestConfigWithTmpKeyFile()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(keyFile.Name())

	cfgFile, err := writeTestConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(cfgFile.Name())

	// Temporarily set $HOME to temp directory to bypass default
	// access config loading.

	tmpHome, err := ioutil.TempDir("", "packer_config_test")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}
	defer os.Remove(tmpHome)

	home := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", home)

	// Config tests
	//TODO (HarveyLowndes) missing credentials
	/*t.Run("BaseConfig", func(t *testing.T) {
		raw := testConfig(cfgFile)
		_, errs := NewConfig(raw)

		if errs != nil {
			t.Fatalf("err: %+v", errs)
		}

	})*/

	//TODO (HarveyLowndes) I dont know how relevant this is?
	t.Run("NoAccessConfig", func(t *testing.T) {
		raw := testConfig(cfgFile)
		delete(raw, "access_cfg_file")
		//Test fails unless i have the following
		delete(raw, "user_ocid")
		delete(raw, "tenancy_ocid")
		delete(raw, "fingerprint")

		_, errs := NewConfig(raw)

		expectedErrors := []string{
			"'user_ocid'", "'tenancy_ocid'", "'fingerprint'",
			//"'key_file'",
		}

		s := errs.Error()

		for _, expected := range expectedErrors {
			if !strings.Contains(s, expected) {
				t.Errorf("Expected %s to contain '%s'", s, expected)
			}
		}
	})

	t.Run("AccessConfigTemplateOnly", func(t *testing.T) {
		raw := testConfig(cfgFile)
		delete(raw, "access_cfg_file")
		raw["user_ocid"] = "ocid1..."
		raw["tenancy_ocid"] = "ocid1..."
		raw["fingerprint"] = "00:00..."
		raw["region"] = "us-ashburn-1"
		raw["key_file"] = keyFile.Name()

		_, errs := NewConfig(raw)

		if errs != nil {
			t.Fatalf("err: %+v", errs)
		}

	})

	//TODO (HarveyLowndes) missing credentials
	t.Run("TenancyReadFromAccessCfgFile", func(t *testing.T) {
		raw := testConfig(cfgFile)
		c, errs := NewConfig(raw)
		if errs != nil {
			t.Fatalf("err: %+v", errs)
		}

		tenancy, err := c.AccessCfg.TenancyOCID()

		if err != nil {
			t.Fatalf("Unexpected error getting tenancy ocid: %v", err)
		}

		expected := "ocid1.tenancy.oc1..aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		if tenancy != expected {
			t.Errorf("Expected tenancy: %s, got %s.", expected, tenancy)
		}

	})

	//TODO (HarveyLowndes) missing credentials
	t.Run("RegionNotDefaultedToPHXWhenSetInOCISettings", func(t *testing.T) {
		raw := testConfig(cfgFile)
		c, errs := NewConfig(raw)
		if errs != nil {
			t.Fatalf("err: %+v", errs)
		}

		region, err := c.AccessCfg.Region()

		if err != nil {
			t.Fatalf("Unexpected error getting region: %v", err)
		}

		expected := "us-ashburn-1"
		if region != expected {
			t.Errorf("Expected region: %s, got %s.", expected, region)
		}

	})

	// Test the correct errors are produced when required template keys are
	// omitted.
	//TODO (HarveyLowndes) missing credentials
	/*requiredKeys := []string{"availability_domain", "base_image_ocid", "shape", "subnet_ocid"}
	for _, k := range requiredKeys {
		t.Run(k+"_required", func(t *testing.T) {
			raw := testConfig(cfgFile)
			delete(raw, k)

			_, errs := NewConfig(raw)

			if !strings.Contains(errs.Error(), k) {
				t.Errorf("Expected '%s' to contain '%s'", errs.Error(), k)
			}
		})
	}*/

	t.Run("ImageNameDefaultedIfEmpty", func(t *testing.T) {
		raw := testConfig(cfgFile)
		delete(raw, "image_name")

		c, errs := NewConfig(raw)
		if errs != nil {
			t.Fatalf("Unexpected error(s): %s", errs)
		}

		if !strings.Contains(c.ImageName, "packer-") {
			t.Errorf("got default ImageName %q, want image name 'packer-{{timestamp}}'", c.ImageName)
		}
	})

	// Test that AccessCfgFile properties are overridden by their
	// corresponding template keys.
	/*accessOverrides := map[string]string{
		"user_ocid":    "User",
		"tenancy_ocid": "Tenancy",
		"region":       "Region",
		"fingerprint":  "Fingerprint",
	}
	for k, v := range accessOverrides {
		t.Run("AccessCfg."+v+"Overridden", func(t *testing.T) {
			expected := "override"

			raw := testConfig(cfgFile)
			raw[k] = expected

			c, errs := NewConfig(raw)
			if errs != nil {
				t.Fatalf("err: %+v", errs)
			}

			accessVal := getField(&c.AccessCfg, v)
			if accessVal != expected {
				t.Errorf("Expected AccessCfg.%s: %s, got %s", v, expected, accessVal)
			}
		})
	}*/
}

// BaseTestConfig creates the base (DEFAULT) config including a temporary key
// file.
// NOTE: Caller is responsible for removing temporary key file.
func baseTestConfigWithTmpKeyFile() (*ini.File, *os.File, error) {
	keyFile, err := generateRSAKeyFile()
	if err != nil {
		return nil, keyFile, err
	}
	// Build ini
	cfg := ini.Empty()
	section, _ := cfg.NewSection("DEFAULT")
	section.NewKey("region", "us-ashburn-1")
	section.NewKey("tenancy_ocid", "ocid1.tenancy.oc1..aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	section.NewKey("user_ocid", "ocid1.user.oc1..aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	section.NewKey("fingerprint", "70:04:5z:b3:19:ab:90:75:a4:1f:50:d4:c7:c3:33:20")
	section.NewKey("key_file", keyFile.Name())

	return cfg, keyFile, nil
}

// WriteTestConfig writes a ini.File to a temporary file for use in unit tests.
// NOTE: Caller is responsible for removing temporary file.
func writeTestConfig(cfg *ini.File) (*os.File, error) {
	confFile, err := ioutil.TempFile("", "config_file")
	if err != nil {
		return nil, err
	}

	_, err = cfg.WriteTo(confFile)
	if err != nil {
		os.Remove(confFile.Name())
		return nil, err
	}

	return confFile, nil
}

// generateRSAKeyFile generates an RSA key file for use in unit tests.
// NOTE: The caller is responsible for deleting the temporary file.
func generateRSAKeyFile() (*os.File, error) {
	// Create temporary file for the key
	f, err := ioutil.TempFile("", "key")
	if err != nil {
		return nil, err
	}

	// Generate key
	priv, err := rsa.GenerateKey(rand.Reader, 2014)
	if err != nil {
		return nil, err
	}

	// ASN.1 DER encoded form
	privDer := x509.MarshalPKCS1PrivateKey(priv)
	privBlk := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDer,
	}

	// Write the key out
	if _, err := f.Write(pem.EncodeToMemory(&privBlk)); err != nil {
		return nil, err
	}

	return f, nil
}
