package writers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/plentico/plenti/common"
	"github.com/plentico/plenti/readers"
)

// SetSiteConfig writes values to the site's configuration file.
func SetSiteConfig(siteConfig readers.SiteConfig, configPath string) error {

	result, err := json.MarshalIndent(siteConfig, "", "\t")
	if err != nil {
		return fmt.Errorf("Unable to marshal JSON: %v%s", err, common.Caller())

	}

	// Write values to site config file for the project.
	err = ioutil.WriteFile(configPath, result, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Unable to write to config file: %w%s\n", err, common.Caller())

	}
	return nil
}
