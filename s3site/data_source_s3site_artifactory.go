package s3site

import (
	"fmt"
	"github.com/hashicorp/terraform/helper/schema"
	"io"
	"net/http"
	"os"
)

func dataSourceArtifactory() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceArtifactoryRead,

		Schema: map[string]*schema.Schema{
			"username": {
				Type:     schema.TypeString,
				Required: true,
			},
			"password": {
				Type:     schema.TypeString,
				Required: true,
			},
			"repository": {
				Type:     schema.TypeString,
				Required: true,
			},
			"artifact": {
				Type:     schema.TypeString,
				Required: true,
			},
			"path": {
				Type:     schema.TypeString,
				Computed: true,
			},
			// "files": {
			// 	Type:     schema.TypeMap,
			// 	Computed: true,
			// },
		},
	}
}

func dataSourceArtifactoryRead(data *schema.ResourceData, meta interface{}) error {
	data.SetId(fmt.Sprintf("%s/%s", data.Get("repository").(string), data.Get("artifact").(string)))

	if err := prepareTmp(); err != nil {
		return err
	}

	// defer cleanTmp()

	username := data.Get("username").(string)
	password := data.Get("password").(string)
	repository := data.Get("repository").(string)
	artifact := data.Get("artifact").(string)

	url := fmt.Sprintf("%s/%s", repository, artifact)
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	request.SetBasicAuth(username, password)

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Error downloading artifact. HttpStatusCode=%d", response.StatusCode)
	}

	localArtifactPath := fmt.Sprintf("%s/%s", tempDir, artifact)
	if file, err := os.Create(localArtifactPath); err != nil {
		return err
	} else {
		io.Copy(file, response.Body)
		file.Close()
	}

	data.Set("path", localArtifactPath)

	return nil
}
