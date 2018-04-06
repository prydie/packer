package oci

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hashicorp/packer/common"
	"github.com/hashicorp/packer/helper/communicator"
	"github.com/hashicorp/packer/helper/config"
	"github.com/hashicorp/packer/packer"
	"github.com/hashicorp/packer/template/interpolate"
	conf "github.com/oracle/oci-go-sdk/common"

	"github.com/mitchellh/go-homedir"
)

type Config struct {
	common.PackerConfig `mapstructure:",squash"`
	Comm                communicator.Config `mapstructure:",squash"`

	AccessCfg conf.ConfigurationProvider

	AccessCfgFile        string `mapstructure:"access_cfg_file"`
	AccessCfgFileAccount string `mapstructure:"access_cfg_file_account"`

	// Access config overrides
	UserID       string `mapstructure:"user_ocid"`
	TenancyID    string `mapstructure:"tenancy_ocid"`
	Region       string `mapstructure:"region"`
	Fingerprint  string `mapstructure:"fingerprint"`
	KeyFile      string `mapstructure:"key_file"`
	PassPhrase   string `mapstructure:"pass_phrase"`
	UsePrivateIP bool   `mapstructure:"use_private_ip"`

	AvailabilityDomain string `mapstructure:"availability_domain"`
	CompartmentID      string `mapstructure:"compartment_ocid"`

	// Image
	BaseImageID string `mapstructure:"base_image_ocid"`
	Shape       string `mapstructure:"shape"`
	ImageName   string `mapstructure:"image_name"`

	// Networking
	SubnetID string `mapstructure:"subnet_ocid"`

	ctx interpolate.Context
}

func NewConfig(raws ...interface{}) (*Config, error) {
	c := &Config{}

	// Decode from template
	err := config.Decode(c, &config.DecodeOpts{
		Interpolate:        true,
		InterpolateContext: &c.ctx,
	}, raws...)
	if err != nil {
		return nil, fmt.Errorf("Failed to mapstructure Config: %+v", err)
	}

	// Determine where the SDK config is located
	if c.AccessCfgFile == "" {
		c.AccessCfgFile, err = getDefaultOCISettingsPath()
		if err != nil {
			//TODO (HarveyLowndes) log error
		}
	}

	if c.AccessCfgFileAccount == "" {
		c.AccessCfgFileAccount = "DEFAULT"
	}

	var keyContent []byte
	if c.KeyFile != "" {
		// Load private key from disk
		// Expand '~' to $HOME
		path, err := homedir.Expand(c.KeyFile)
		if err != nil {
			return nil, err
		}

		// Read API signing key
		keyContent, err = ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}

	providers := []conf.ConfigurationProvider{
		conf.NewRawConfigurationProvider(c.TenancyID, c.UserID, c.Region, c.Fingerprint, string(keyContent), &c.PassPhrase),
	}

	fileProvider, err := conf.ConfigurationProviderFromFileWithProfile(c.AccessCfgFile, c.AccessCfgFileAccount, c.PassPhrase)
	if err == nil {
		providers = append(providers, fileProvider)
	}

	// Load API access configuration from SDK
	accessCfg, err := conf.ComposingConfigurationProvider(providers)
	if err != nil {
		return nil, err
	}

	var errs *packer.MultiError
	if es := c.Comm.Prepare(&c.ctx); len(es) > 0 {
		errs = packer.MultiErrorAppend(errs, es...)
	}

	if c.ImageName == "" {
		name, err := interpolate.Render("packer-{{timestamp}}", nil)
		if err != nil {
			errs = packer.MultiErrorAppend(errs,
				fmt.Errorf("unable to parse image name: %s", err))
		} else {
			c.ImageName = name
		}
	}

	userOCID, _ := accessCfg.UserOCID()
	if userOCID == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("'user_ocid' must be specified"))
	}

	tenancyOCID, _ := accessCfg.TenancyOCID()
	if tenancyOCID == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("'tenancy_ocid' must be specified"))
	}

	region, _ := accessCfg.Region()
	if region == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("'region' must be specified"))
	}

	fingerprint, _ := accessCfg.KeyFingerprint()
	if fingerprint == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("'fingerprint' must be specified"))
	}

	//TODO (HarveyLowndes) when does this condition occur?
	privateKey := accessCfg.PrivateRSAKey
	if privateKey == nil {
		errs = packer.MultiErrorAppend(
			errs, errors.New("'PrivateRSAKey' must be specified"))
	}

	c.AccessCfg = accessCfg

	if errs != nil && len(errs.Errors) > 0 {
		return nil, errs
	}

	return c, nil
}

// getDefaultOCISettingsPath uses mitchellh/go-homedir to compute the default
// config file location ($HOME/.oci/config).
func getDefaultOCISettingsPath() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(home, ".oci", "config")
	if _, err := os.Stat(path); err != nil {
		return "", err
	}

	return path, nil
}
