// Copyright 2021, The Go Authors. All rights reserved.
// Author: crochee
// Date: 2021/1/30

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"proxy-go/api/response"
	"proxy-go/config/dynamic"
	"proxy-go/middlewares"
	"proxy-go/server"
)

func UpdateSwitch(ctx *gin.Context) {
	var dynamicSwitch dynamic.Switch
	if err := ctx.ShouldBindBodyWith(&dynamicSwitch, binding.JSON); err != nil {
		response.GinError(ctx, response.ErrorWith(http.StatusBadRequest, err))
		return
	}
	if server.GlobalWatcher == nil {
		response.ErrorWithMessage(ctx, "please check server")
		return
	}
	server.GlobalWatcher.Entry() <- &server.Message{
		Name: middlewares.Switcher,
		Content: &dynamic.Config{
			Switcher: &dynamicSwitch,
		},
	}
	ctx.Status(http.StatusOK)
}
