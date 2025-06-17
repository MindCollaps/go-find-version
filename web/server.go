package web

import (
	"github.com/gin-gonic/gin"
	"strconv"
)

func Init(port int) {
	r := gin.Default()

	sPort := strconv.Itoa(port)

	r.Run(":" + sPort)
}
