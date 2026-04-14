package main

import (
  "context"
  "fmt"
  "net"
  "os"

  "github.com/danielgtaylor/huma/v2"
  "github.com/pemako/mk/app"
  "github.com/pemako/mk/config"
  "github.com/pemako/mk/errors"
  "github.com/pemako/mk/logger"
  httpserver "github.com/pemako/mk/server/http"
)

// --- Input / Output types ---

type GreetInput struct {
  Name string `path:"name" minLength:"1" maxLength:"50" doc:"Name to greet" example:"world"`
}

type GreetOutput struct {
  Body struct {
    Message string `json:"message" doc:"Greeting message"`
  }
}

type CreateNoteInput struct {
  Body struct {
    Title   string `json:"title"   minLength:"1" maxLength:"100" doc:"Note title"`
    Content string `json:"content" minLength:"1" maxLength:"5000" doc:"Note body"`
    Tags    []string `json:"tags,omitempty" maxItems:"10" doc:"Optional labels"`
  }
}

type NoteOutput struct {
  Body struct {
    ID      string   `json:"id"      doc:"Generated note ID"`
    Title   string   `json:"title"   doc:"Note title"`
    Content string   `json:"content" doc:"Note body"`
    Tags    []string `json:"tags"    doc:"Labels"`
  }
}

type ErrorDemoInput struct {
  Code int `query:"code" minimum:"400" maximum:"599" doc:"HTTP status code to simulate" example:"404"`
}

// --- Handlers ---

func greet(_ context.Context, input *GreetInput) (*GreetOutput, error) {
  out := &GreetOutput{}
  out.Body.Message = "Hello, " + input.Name + "!"
  return out, nil
}

func createNote(_ context.Context, input *CreateNoteInput) (*NoteOutput, error) {
  out := &NoteOutput{}
  out.Body.ID = "note_001"
  out.Body.Title = input.Body.Title
  out.Body.Content = input.Body.Content
  out.Body.Tags = input.Body.Tags
  return out, nil
}

func errorDemo(_ context.Context, input *ErrorDemoInput) (*struct{}, error) {
  switch input.Code {
  case 400:
    return nil, huma.Error400BadRequest("bad request demo", errors.ErrBadRequest)
  case 401:
    return nil, huma.Error401Unauthorized("unauthorized demo", errors.ErrUnauthorized)
  case 403:
    return nil, huma.Error403Forbidden("forbidden demo", errors.ErrForbidden)
  case 404:
    return nil, huma.Error404NotFound("not found demo", errors.ErrNotFound)
  default:
    return nil, huma.Error500InternalServerError("internal error demo", errors.ErrInternal)
  }
}

// --- main ---

func main() {
  cfg, err := config.Load()
  if err != nil {
    fmt.Fprintf(os.Stderr, "config: %v\n", err)
    os.Exit(1)
  }

  log, err := logger.New(logger.WithConfig(cfg))
  if err != nil {
    fmt.Fprintf(os.Stderr, "logger: %v\n", err)
    os.Exit(1)
  }

  // Bind to a random port to avoid conflicts in development.
  ln, err := net.Listen("tcp", ":8080")
  if err != nil {
    fmt.Fprintf(os.Stderr, "listen: %v\n", err)
    os.Exit(1)
  }
  log.Info(context.Background(), "quickstart listening",
    logger.F("addr", ln.Addr().String()),
    logger.F("docs", "http://"+ln.Addr().String()+"/docs"),
  )

  srv, err := httpserver.New(
    httpserver.WithConfig(cfg),
    httpserver.WithListener(ln),
    httpserver.WithLogger(log),
    // Enable Huma: mounts OpenAPI spec + Swagger UI at /docs.
    httpserver.WithOpenAPI("Quickstart API", "1.0.0", "/docs"),
  )
  if err != nil {
    fmt.Fprintf(os.Stderr, "http server: %v\n", err)
    os.Exit(1)
  }

  api := srv.API()

  huma.Register(api, huma.Operation{
    Method:      "GET",
    Path:        "/hello/{name}",
    Summary:     "Greet someone",
    Description: "Returns a personalised greeting for the given name.",
    Tags:        []string{"greetings"},
  }, greet)

  huma.Register(api, huma.Operation{
    Method:        "POST",
    Path:          "/notes",
    Summary:       "Create a note",
    Description:   "Creates a new note with title, content, and optional tags.",
    Tags:          []string{"notes"},
    DefaultStatus: 201,
  }, createNote)

  huma.Register(api, huma.Operation{
    Method:      "GET",
    Path:        "/error-demo",
    Summary:     "Simulate an error response",
    Description: "Pass ?code=404 (or 400/401/403/500) to see structured error output.",
    Tags:        []string{"demo"},
  }, errorDemo)

  if err := app.New(
    app.WithName("quickstart"),
    app.WithLogger(log),
    app.WithServer(srv),
  ).Run(); err != nil {
    log.Error(context.Background(), "run failed", logger.F("err", err))
  }
}
