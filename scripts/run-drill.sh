#!/bin/sh
set -eu

curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"project_id":"analytics","name":"etl-rotation-drill","grace_hours":24,"build":{"build_id":"build-417","deployment":"worker-blue"}}' \
  "${ROTATION_SERVICE_URL:-http://localhost:8080}/rotation-drills"
