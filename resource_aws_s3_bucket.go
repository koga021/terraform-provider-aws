package aws

import (
	"fmt"
	"log"

	"github.com/hashicorp/terraform/helper/schema"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/service/s3"
)

func resourceAwsS3Bucket() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsS3BucketCreate,
		Read:   resourceAwsS3BucketRead,
		Update: resourceAwsS3BucketUpdate,
		Delete: resourceAwsS3BucketDelete,

		Schema: map[string]*schema.Schema{
			"bucket": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"acl": &schema.Schema{
				Type:     schema.TypeString,
				Default:  "private",
				Optional: true,
				ForceNew: true,
			},

			"website": &schema.Schema{
				Type:     schema.TypeBool,
				Default:  false,
				Optional: true,
				ForceNew: false,
			},

			"index_document": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"error_document": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"tags": tagsSchema(),
		},
	}
}

func resourceAwsS3BucketCreate(d *schema.ResourceData, meta interface{}) error {
	s3conn := meta.(*AWSClient).s3conn
	awsRegion := meta.(*AWSClient).region

	// Get the bucket and acl
	bucket := d.Get("bucket").(string)
	acl := d.Get("acl").(string)

	log.Printf("[DEBUG] S3 bucket create: %s, ACL: %s", bucket, acl)

	req := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
		ACL:    aws.String(acl),
	}

	// Special case us-east-1 region and do not set the LocationConstraint.
	// See "Request Elements: http://docs.aws.amazon.com/AmazonS3/latest/API/RESTBucketPUT.html
	if awsRegion != "us-east-1" {
		req.CreateBucketConfiguration = &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(awsRegion),
		}
	}

	_, err := s3conn.CreateBucket(req)
	if err != nil {
		return fmt.Errorf("Error creating S3 bucket: %s", err)
	}

	// Assign the bucket name as the resource ID
	d.SetId(bucket)

	return resourceAwsS3BucketUpdate(d, meta)
}

func resourceAwsS3BucketUpdate(d *schema.ResourceData, meta interface{}) error {
	s3conn := meta.(*AWSClient).s3conn
	if err := setTagsS3(s3conn, d); err != nil {
		return err
	}

	if err := updateWebsite(s3conn, d); err != nil {
		return err
	}

	return resourceAwsS3BucketRead(d, meta)
}

func resourceAwsS3BucketRead(d *schema.ResourceData, meta interface{}) error {
	s3conn := meta.(*AWSClient).s3conn

	_, err := s3conn.HeadBucket(&s3.HeadBucketInput{
		Bucket: aws.String(d.Id()),
	})
	if err != nil {
		if awsError, ok := err.(aws.APIError); ok && awsError.StatusCode == 404 {
			d.SetId("")
		} else {
			// some of the AWS SDK's errors can be empty strings, so let's add
			// some additional context.
			return fmt.Errorf("error reading S3 bucket \"%s\": %s", d.Id(), err)
		}
	}

	tagSet, err := getTagSetS3(s3conn, d.Id())
	if err != nil {
		return err
	}

	if err := d.Set("tags", tagsToMapS3(tagSet)); err != nil {
		return err
	}

	return nil
}

func resourceAwsS3BucketDelete(d *schema.ResourceData, meta interface{}) error {
	s3conn := meta.(*AWSClient).s3conn

	log.Printf("[DEBUG] S3 Delete Bucket: %s", d.Id())
	_, err := s3conn.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(d.Id()),
	})
	if err != nil {
		return err
	}
	return nil
}

func updateWebsite(s3conn *s3.S3, d *schema.ResourceData) error {
	website := d.Get("website").(bool)
	bucket := d.Get("bucket").(string)
	indexDocument := d.Get("index_document").(string)
	errorDocument := d.Get("error_document").(string)

	if website {
		websiteConfiguration := &s3.WebsiteConfiguration{}

		if indexDocument != "" {
			websiteConfiguration.IndexDocument = &s3.IndexDocument{Suffix: aws.String(indexDocument)}
		}

		if errorDocument != "" {
			websiteConfiguration.ErrorDocument = &s3.ErrorDocument{Key: aws.String(errorDocument)}
		}

		putInput := &s3.PutBucketWebsiteInput{
			Bucket:               aws.String(bucket),
			WebsiteConfiguration: websiteConfiguration,
		}

		log.Printf("[DEBUG] S3 put bucket website: %s", putInput)

		_, err := s3conn.PutBucketWebsite(putInput)
		if err != nil {
			return fmt.Errorf("Error putting S3 website: %s", err)
		}
	} else {
		deleteInput := &s3.DeleteBucketWebsiteInput{Bucket: aws.String(bucket)}

		log.Printf("[DEBUG] S3 delete bucket website: %s", deleteInput)

		_, err := s3conn.DeleteBucketWebsite(deleteInput)
		if err != nil {
			return fmt.Errorf("Error deleting S3 website: %s", err)
		}
	}

	return nil
}
