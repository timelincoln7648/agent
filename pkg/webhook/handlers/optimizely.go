/****************************************************************************
 * Copyright 2019, Optimizely, Inc. and contributors                        *
 *                                                                          *
 * Licensed under the Apache License, Version 2.0 (the "License");          *
 * you may not use this file except in compliance with the License.         *
 * You may obtain a copy of the License at                                  *
 *                                                                          *
 *    http://www.apache.org/licenses/LICENSE-2.0                            *
 *                                                                          *
 * Unless required by applicable law or agreed to in writing, software      *
 * distributed under the License is distributed on an "AS IS" BASIS,        *
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. *
 * See the License for the specific language governing permissions and      *
 * limitations under the License.                                           *
 ***************************************************************************/

// Package handlers //
package handlers

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/go-chi/render"
	"github.com/rs/zerolog/log"

	"github.com/optimizely/sidedoor/pkg/optimizely"

	"github.com/optimizely/sidedoor/pkg/webhook/models"
)

const signatureHeader = "X-Hub-Signature"
const signaturePrefix = "sha1="
const defaultWebhookConfigFile = "config.yaml"


// OptlyWebhookHandler handles incoming messages from Optimizely
type OptlyWebhookHandler struct{
	Configs			[]models.OptlyWebhookConfig
	optlyClient		optimizely.OptlyClient
}

// computeSignature computes signature based on payload
func (h *OptlyWebhookHandler)computeSignature(payload []byte, secretKey string) string {
	mac := hmac.New(sha1.New, []byte(secretKey))
	_, err := mac.Write(payload)

	if err != nil {
		log.Error().Msg("Unable to compute signature.")
		return ""
	}

	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// validateSignature computes and compares message digest
func (h *OptlyWebhookHandler) validateSignature(requestSignature string, payload []byte) bool {
	// TODO change this to fetch secret based on project ID
	secretKey := h.Configs[0].Secret
	computedSignature := h.computeSignature(payload, secretKey)
	return subtle.ConstantTimeCompare([]byte(computedSignature), []byte(requestSignature)) == 1
}

// Init initializes the webhook handler with all known webhooks
func (h *OptlyWebhookHandler) Init() {
	configFile, configFileSet := os.LookupEnv("OPTLY_WEBHOOK_CONFIG_FILE")
	if !configFileSet {
		configFile = defaultWebhookConfigFile
	}
	webhooksSource, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Error().Msg("Unable to read config file.")
		panic(err)
	}

	err = yaml.Unmarshal(webhooksSource, &h.Configs)
	if err != nil {
		log.Error().Msg("Unable to parse config file.")
		panic(err)
	}
}

// HandleWebhook handles incoming webhook messages from Optimizely application
func (h *OptlyWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request)  {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error().Msg("Unable to read webhook message body.")
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{
			"error": "Unable to read webhook message body.",
		})
		return
	}

	var webhookMsg models.OptlyMessage
	err = json.Unmarshal(body, &webhookMsg)
	if err != nil {
		log.Error().Msg("Unable to parse webhook message.")
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{
			"error": "Unable to parse webhook message.",
		})
		return
	}

	requestSignature := r.Header.Get(signatureHeader)
	isValid := h.validateSignature(requestSignature, body)

	if !isValid {
		log.Error().Msg("Computed signature does not match signature in request. Ignoring message.")
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{
			"error": "Computed signature does not match signature in request. Ignoring message.",
		})
		return
	}

	h.optlyClient.UpdateConfig()
	w.WriteHeader(http.StatusNoContent)
}