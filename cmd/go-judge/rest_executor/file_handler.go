package restexecutor

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
	"github.com/gin-gonic/gin"
)

type FileHandler struct {
	fs filestore.FileStore
}

func NewFileHandler(fs filestore.FileStore) *FileHandler {
	return &FileHandler{
		fs: fs,
	}
}

func (f *FileHandler) fileGet(c *gin.Context) {
	ids := f.fs.List()
	c.JSON(http.StatusOK, ids)
}

func (f *FileHandler) filePost(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	fi, err := fh.Open()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	sf, err := f.fs.New()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	defer sf.Close()

	if _, err := sf.ReadFrom(fi); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	id, err := f.fs.Add(fh.Filename, sf.Name())
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, id)
}

func (f *FileHandler) fileIDGet(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	name, file := f.fs.Get(uri.FileID)
	if file == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	typ := mime.TypeByExtension(path.Ext(name))
	c.Header("Content-Type", typ)

	fi, ok := file.(*envexec.FileInput) // fast path
	if ok {
		c.FileAttachment(fi.Path, name)
		return
	}

	r, err := envexec.FileToReader(file)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	defer r.Close()

	content, err := io.ReadAll(r)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))
	c.Data(http.StatusOK, typ, content)
}

func (f *FileHandler) fileIDDelete(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	ok := f.fs.Remove(uri.FileID)
	if !ok {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}
