package ws

import (
	"github.com/ClusterOperator/kubepi/pkg/logging"
	"github.com/ClusterOperator/kubepi/pkg/terminal"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/context"
)

func Install(parent iris.Party) {
	wsParty := parent.Party("/ws")
	h := terminal.CreateAttachHandler("/terminal/sockjs")
	wsParty.Any("/terminal/sockjs/{p:path}", func(ctx *context.Context) {
		h.ServeHTTP(ctx.ResponseWriter(), ctx.Request())
	})
	l := logging.CreateLoggingHandler("logging/sockjs")
	wsParty.Any("/logging/sockjs/{p:path}", func(ctx *context.Context) {
		l.ServeHTTP(ctx.ResponseWriter(), ctx.Request())
	})
}
