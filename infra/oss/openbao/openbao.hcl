# OpenBao is private to the Compose `secrets` network, which only the API
# joins. This listener uses HTTP only there. There is no web UI.
ui = false
# Integrated Raft storage does not support mlock well. Keep host swap off or
# encrypted so OpenBao memory never reaches disk in clear text.
disable_mlock = true
api_addr = "http://openbao:8200"
cluster_addr = "http://openbao:8201"

storage "raft" {
  path = "/openbao/data"
  node_id = "voxis-oss"
}

listener "tcp" {
  address = "0.0.0.0:8200"
  tls_disable = 1
  # OpenBao 2.6 already defaults both to true. They are pinned here so the
  # API-facing listener never serves the unauthenticated root-generation or
  # rekey endpoints, even if a future default changes.
  disable_unauthed_generate_root_endpoints = true
  disable_unauthed_rekey_endpoints = true
}

# Root-token generation from unseal shares (restore and AppRole rotation) is
# reachable only from inside this container, through `docker compose exec`.
listener "tcp" {
  address = "127.0.0.1:8210"
  tls_disable = 1
  disable_unauthed_generate_root_endpoints = false
}

# Every request and response is audited, with secrets HMAC-hashed. OpenBao
# refuses requests when it cannot write this log, so watch the free space of
# OPENBAO_DATA_DIR and rotate the file (see README).
audit "file" "file" {
  options {
    file_path = "/openbao/data/audit.log"
  }
}
