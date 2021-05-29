// Copyright (c) Huawei Technologies Co., Ltd. 2021-2021. All rights reserved.
// Description:
// Author: licongfu
// Create: 2021/5/29

// Package main
package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestServer(t *testing.T) {
	serverCmd := &cobra.Command{
		Use:    "server",
		Short:  "start server",
		Long:   "start multi server",
		RunE:   server,
		Hidden: true,
	}
	serverCmd.Flags().StringP("config", "c", "../../conf/config.yml", "")
	serverCmd.SetErr(bytes.NewBuffer(nil))
	if err := serverCmd.Execute(); err != nil {
		t.Log(err)
	}

}
