package dashboard

import "embed"

//go:generate sh -c "cd ../../web && npm install && npm run build && rm -f ../internal/dashboard/static/index.html && cp -r build/* ../internal/dashboard/static/"

//go:embed all:static
var frontendFS embed.FS
