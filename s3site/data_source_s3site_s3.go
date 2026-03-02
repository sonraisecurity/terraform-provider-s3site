package s3site

import (
	"fmt"
	"path/filepath"

	"github.com/hashicorp/terraform/helper/schema"
)

func dataSourceS3() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceS3Read,

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The S3 bucket containing the artifact.",
			},
			"key": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The S3 object key of the artifact.",
			},
			"path": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The local filesystem path where the artifact was downloaded.",
			},
		},
	}
}

func dataSourceS3Read(data *schema.ResourceData, meta interface{}) error {
	s3Helper := meta.(*Meta).S3Helper

	bucket := data.Get("bucket").(string)
	key := data.Get("key").(string)

	data.SetId(fmt.Sprintf("s3://%s/%s", bucket, key))

	if err := prepareTmp(); err != nil {
		return err
	}

	localPath := fmt.Sprintf("%s/%s", tempDir, filepath.Base(key))

	if err := s3Helper.GetObject(bucket, key, localPath); err != nil {
		return err
	}

	data.Set("path", localPath)

	return nil
}
