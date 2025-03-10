provider "ironic" {
  url          = "http://localhost:6385/v1"
  inspector    = "http://localhost:5050/v1"
  microversion = "1.52"
  timeout      = 900
}
