/*
Copyright (C) 2018 Expedia Group.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"github.com/ExpediaGroup/flyte-slack/cache"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
	"strconv"
	"time"
)

const (
	tokenEnvKey           = "FLYTE_SLACK_TOKEN"
	packNameKey           = "PACK_NAME"
	logLevelKey           = "LOGLEVEL"
	renewConversationList = "RENEW_CONVERSATION_LIST" // how often conversation list is updated  cache (hours)
)

func logLevel() zerolog.Level {
	ll := getEnvDefault(logLevelKey, "debug")
	l, err := zerolog.ParseLevel(ll)
	if err != nil {
		log.Err(err).Msg("Unable to parse log level setting to Debug")
		return zerolog.DebugLevel
	}
	return l
}

func packName() string {
	return getEnvDefault(packNameKey, "Slack")
}

func slackToken() string {
	return getEnv(tokenEnvKey, true)
}

func cacheConfig() (*cache.Config, error) {
	rc := getEnvDefault(renewConversationList, "24")

	t, err := strconv.Atoi(rc)
	if err != nil {
		return nil, err
	}

	return &cache.Config{
		RenewConversationListFrequency: time.Duration(t) * time.Hour,
	}, nil
}

func getEnv(key string, required bool) string {

	if v, _ := os.LookupEnv(key); v != "" {
		return v
	}

	if required {
		log.Fatal().Msgf("env=%s not set", key)
	}
	return ""
}

func getEnvDefault(key string, def string) string {
	if v, _ := os.LookupEnv(key); v != "" {
		return v
	}

	return def
}
