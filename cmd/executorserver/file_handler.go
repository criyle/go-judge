package main

import (
	"io/ioutil"
	"mime"
	"net/http"
	"path"

	"github.com/gin-gonic/gin"
)

func fileGet(c *gin.Context) {
	ids := getAllFileID()
	c.JSON(http.StatusOK, ids)
}

func filePost(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	f, err := fh.Open()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	b, err := ioutil.ReadAll(f)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	id, err := addFile(b, fh.Filename)

	c.JSON(http.StatusOK, id)
}

func fileIDGet(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	f, ok := getFile(uri.FileID)
	if !ok {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	typ := mime.TypeByExtension(path.Ext(f.FileName))
	c.Data(http.StatusOK, typ, f.Content)
}

func fileIDDelete(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	ok := removeFile(uri.FileID)
	if !ok {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}
