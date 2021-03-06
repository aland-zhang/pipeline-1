/*
 * Pipeline API
 *
 * Pipeline is a feature rich application platform, built for containers on top of Kubernetes to automate the DevOps experience, continuous application development and the lifecycle of deployments. 
 *
 * API version: latest
 * Contact: info@banzaicloud.com
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package pipeline

type CreateSecretRequest struct {

	Name string `json:"name"`

	Type string `json:"type"`

	Tags []string `json:"tags,omitempty"`

	Values map[string]interface{} `json:"values"`
}
