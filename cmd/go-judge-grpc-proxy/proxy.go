// Command executorclient is used to test executor server's grpc call
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/criyle/go-judge/pb"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	addr    = flag.String("addr", ":7755", "Rest api server addr")
	srvAddr = flag.String("srvaddr", "localhost:5051", "GRPC server addr")
)

type execProxy struct {
	client pb.ExecutorClient
}

func (p *execProxy) Exec(c *gin.Context) {
	req := new(pb.Request)
	b, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	if err := protojson.Unmarshal(b, req); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	log.Println(req)
	rep, err := p.client.Exec(c, req)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (p *execProxy) FileList(c *gin.Context) {
	rep, err := p.client.FileList(c, &emptypb.Empty{})
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (p *execProxy) FileGet(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	fid := &pb.FileID{
		FileID: uri.FileID,
	}
	rep, err := p.client.FileGet(c, fid)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (p *execProxy) FilePost(c *gin.Context) {
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
	b, err := io.ReadAll(fi)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	req := &pb.FileContent{
		Name:    fh.Filename,
		Content: b,
	}
	rep, err := p.client.FileAdd(c, req)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (p *execProxy) FileDelete(c *gin.Context) {
	type fileURI struct {
		FileID string `uri:"fid"`
	}
	var uri fileURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	fid := &pb.FileID{
		FileID: uri.FileID,
	}
	rep, err := p.client.FileDelete(c, fid)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

func main() {
	flag.Parse()
	token := os.Getenv("TOKEN")
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(newTokenAuth(token)))
	}
	conn, err := grpc.NewClient(*srvAddr, opts...)
	if err != nil {
		log.Fatalln("client", err)
	}
	client := pb.NewExecutorClient(conn)

	p := &execProxy{client: client}

	r := gin.Default()
	r.POST("/exec", p.Exec)
	r.GET("/file", p.FileList)
	r.GET("/file/:fid", p.FileGet)
	r.POST("/file", p.FilePost)
	r.DELETE("/file/:fid", p.FileDelete)

	log.Println(r.Run(*addr))
}

type tokenAuth struct {
	token string
}

func newTokenAuth(token string) credentials.PerRPCCredentials {
	return &tokenAuth{token: token}
}

// Return value is mapped to request headers.
func (t *tokenAuth) GetRequestMetadata(ctx context.Context, in ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + t.token,
	}, nil
}

func (*tokenAuth) RequireTransportSecurity() bool {
	return false
}
