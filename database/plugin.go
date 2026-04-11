package database

import (
	"context"
	"time"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const pluginName = "mk:tracing"

type tracingPlugin struct {
	dbName        string
	log           core.Logger
	slowThreshold time.Duration
}

func newTracingPlugin(dbName string, log core.Logger, slowThreshold time.Duration) *tracingPlugin {
	if log == nil {
		log = logger.Default()
	}
	if slowThreshold == 0 {
		slowThreshold = 200 * time.Millisecond
	}
	return &tracingPlugin{dbName: dbName, log: log, slowThreshold: slowThreshold}
}

func (p *tracingPlugin) Name() string { return pluginName }

func (p *tracingPlugin) Initialize(db *gorm.DB) error {
	db.Callback().Query().Before("gorm:query").Register(pluginName+":bq", p.before)
	db.Callback().Query().After("gorm:query").Register(pluginName+":aq", p.after)
	db.Callback().Create().Before("gorm:create").Register(pluginName+":bc", p.before)
	db.Callback().Create().After("gorm:create").Register(pluginName+":ac", p.after)
	db.Callback().Update().Before("gorm:update").Register(pluginName+":bu", p.before)
	db.Callback().Update().After("gorm:update").Register(pluginName+":au", p.after)
	db.Callback().Delete().Before("gorm:delete").Register(pluginName+":bd", p.before)
	db.Callback().Delete().After("gorm:delete").Register(pluginName+":ad", p.after)
	return nil
}

type startTimeKey struct{}

func (p *tracingPlugin) before(db *gorm.DB) {
	ctx := db.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		_, childSpan := span.TracerProvider().Tracer("mk/database").Start(ctx, "gorm:"+p.dbName)
		ctx = trace.ContextWithSpan(ctx, childSpan)
	}
	db.Statement.Context = context.WithValue(ctx, startTimeKey{}, time.Now())
}

func (p *tracingPlugin) after(db *gorm.DB) {
	ctx := db.Statement.Context
	if ctx == nil {
		return
	}
	start, _ := ctx.Value(startTimeKey{}).(time.Time)
	elapsed := time.Since(start)

	sql := db.Dialector.Explain(db.Statement.SQL.String(), db.Statement.Vars...)
	rows := db.Statement.RowsAffected

	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("db.statement", sql),
			attribute.Int64("db.rows_affected", rows),
			attribute.String("db.name", p.dbName),
		)
		if db.Error != nil {
			span.RecordError(db.Error)
		}
		span.End()
	}

	p.log.Debug(ctx, "sql",
		core.Field{Key: "db", Value: p.dbName},
		core.Field{Key: "sql", Value: sql},
		core.Field{Key: "rows", Value: rows},
		core.Field{Key: "elapsed", Value: elapsed.String()},
	)
}
