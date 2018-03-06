package s3site

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/hashicorp/terraform/helper/schema"
	"log"
	"time"
)

func resourceCloudfrontInvalidation() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudfrontInvalidationCreate,
		Read:   resourceCloudfrontInvalidationRead,
		Update: resourceCloudfrontInvalidationUpdate,
		Delete: resourceCloudfrontInvalidationDelete,
		Importer: &schema.ResourceImporter{
			State: importState,
		},

		Schema: map[string]*schema.Schema{
			"cloudfront_distribution_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"files": {
				Type:     schema.TypeMap,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceCloudfrontInvalidationCreate(data *schema.ResourceData, meta interface{}) error {
	distributionId := data.Get("cloudfront_distribution_id").(string)
	files := data.Get("files").(map[string]interface{})

	sess := meta.(*session.Session)
	svc := cloudfront.New(sess)

	timestamp := time.Now().String()
	quantity := int64(len(files))

	var items []*string

	for key, _ := range files {
		decodedKey := fmt.Sprintf("/%s", decodeKey(key))
		items = append(items, &decodedKey)
	}

	log.Printf("[INFO] Creating invalidation request. paths=%d", len(files))

	input := &cloudfront.CreateInvalidationInput{
		DistributionId: &distributionId,
		InvalidationBatch: &cloudfront.InvalidationBatch{
			CallerReference: &timestamp,
			Paths: &cloudfront.Paths{
				Items:    items,
				Quantity: &quantity,
			},
		},
	}

	response, err := svc.CreateInvalidation(input)
	if err != nil {
		return err
	}

	data.SetId(*response.Invalidation.Id)

	return nil
}

func resourceCloudfrontInvalidationRead(data *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceCloudfrontInvalidationUpdate(data *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceCloudfrontInvalidationDelete(data *schema.ResourceData, meta interface{}) error {
	return nil
}
