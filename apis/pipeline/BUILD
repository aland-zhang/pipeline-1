subinclude("///pleasings2//openapi")

openapi_library(
    name = "pipeline",
    out = ".gen/pipeline",
    additional_properties = {
        "packageName": "pipeline",
        "withGoCodegenComment": "true",
        "sourceFolder": "pipeline",
    },
    generator = "go-server",
    spec = "pipeline.yaml",
    strip = ["{Dockerfile,go.*,README.md,main.go,pipeline/api*.go,pipeline/logger.go,pipeline/routers.go}"],
)

openapi_validate(
    name = "validate",
    spec = "pipeline.yaml",
)
