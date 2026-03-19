package handlers

import (
	"compress/gzip"
	"io"
	"strings"

	"github.com/Henelik/penbomb/payloads"
	"github.com/gofiber/fiber/v2"
)

// FiberPenbomb returns a zip bomb to the client.
//
// Brotli path: serves the embedded 100 GiB brotli bomb (~79 KiB compressed).
// Sending the pre-built payload has a near-zero server CPU cost and produces the
// highest possible expansion ratio (~1,340,000x).
//
// Gzip fallback: generates a 100 GiB gzip bomb on the fly via an io.Pipe so
// the server never buffers the full payload in memory.
//
// Goroutine lifecycle (gzip path only):
//   - Normal completion: compressor finishes, closes pw, fasthttp reads EOF.
//   - Client disconnect: fasthttp closes pr; next write into pw returns
//     io.ErrClosedPipe; goroutine exits via the CloseWithError path.
//   - Context cancellation: contextReader.Read returns io.ErrClosedPipe on the
//     next read, same exit path.
func FiberPenbomb(ctx *fiber.Ctx) error {
	accept := ctx.Get("Accept-Encoding")

	if strings.Contains(accept, "br") {
		ctx.Set("Content-Encoding", "br")
		ctx.Set("Content-Type", "text/plain")
		// SetBodyRaw writes the pre-compressed bytes verbatim, bypassing any
		// Fiber compression middleware.
		ctx.Response().SetBodyRaw(payloads.Brotli100GiB)
		return nil
	}

	// Gzip fallback: generate on the fly.
	pr, pw := io.Pipe()

	done := ctx.Context().Done()
	src := contextReader{
		r:    io.LimitReader(zeroReader{}, decompressedSize),
		done: done,
	}

	ctx.Set("Content-Encoding", "gzip")
	ctx.Set("Content-Type", "text/plain")

	go func() {
		w, err := gzip.NewWriterLevel(pw, gzip.BestCompression)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_, err = io.Copy(w, src)
		if err != nil {
			_ = w.Close()
			_ = pw.CloseWithError(err)
			return
		}
		if err = w.Close(); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()

	// -1 tells fasthttp the content length is unknown; it streams until EOF.
	ctx.Response().SetBodyStream(pr, -1)
	return nil
}

// RegisterSuggestedRoutes registers endpoints commonly probed by vulnerability
// scanners, pentesters, and automated scrapers that would never be legitimately
// served by a Go application.
//
// Register this LAST so it does not shadow intentional application routes.
// Uses router.All to catch scanners that probe with POST/PUT/HEAD as well as GET.
func RegisterSuggestedRoutes(router *fiber.App) {
	// === WordPress & common PHP CMSes ===
	router.All("/wp-*", FiberPenbomb)       // wp-admin, wp-login.php, wp-config.php, wp-cron.php, etc.
	router.All("/wordpress*", FiberPenbomb) // /wordpress/ installs
	router.All("/joomla*", FiberPenbomb)
	router.All("/drupal*", FiberPenbomb)
	router.All("/magento*", FiberPenbomb)
	router.All("/typo3*", FiberPenbomb)
	router.All("/bitrix*", FiberPenbomb) // Bitrix CMS common in Russia/CIS regions

	// === PHP (any .php on a Go app is illegitimate) ===
	router.All("/*.php", FiberPenbomb) // root-level PHP files

	// === Admin / control panels ===
	router.All("/administrator*", FiberPenbomb) // Joomla admin
	router.All("/phpmyadmin*", FiberPenbomb)
	router.All("/pma*", FiberPenbomb) // phpMyAdmin short alias
	router.All("/myadmin*", FiberPenbomb)
	router.All("/mysql*", FiberPenbomb)
	router.All("/adminer*", FiberPenbomb)
	router.All("/panel*", FiberPenbomb)
	router.All("/cpanel*", FiberPenbomb)
	router.All("/webadmin*", FiberPenbomb)
	router.All("/manager*", FiberPenbomb) // Tomcat manager

	// === Sensitive dotfiles & version control ===
	router.All("/.env*", FiberPenbomb) // .env, .env.local, .env.production, .env.backup
	router.All("/.git*", FiberPenbomb) // .git/, .git/config, .git/HEAD, .gitignore, .gitconfig
	router.All("/.svn*", FiberPenbomb)
	router.All("/.hg*", FiberPenbomb) // Mercurial
	router.All("/.cvs*", FiberPenbomb)
	router.All("/.htaccess", FiberPenbomb)
	router.All("/.htpasswd", FiberPenbomb)
	router.All("/.ssh*", FiberPenbomb)  // .ssh/id_rsa, .ssh/authorized_keys
	router.All("/.aws*", FiberPenbomb)  // .aws/credentials
	router.All("/.kube*", FiberPenbomb) // .kube/config
	router.All("/.config*", FiberPenbomb)
	router.All("/.bashrc", FiberPenbomb)
	router.All("/.bash_history", FiberPenbomb)
	router.All("/.profile", FiberPenbomb)
	router.All("/.passwd", FiberPenbomb)
	router.All("/.DS_Store", FiberPenbomb) // macOS folder metadata
	router.All("/.npmrc", FiberPenbomb)    // npm auth tokens
	router.All("/.pypirc", FiberPenbomb)   // PyPI credentials
	router.All("/.netrc", FiberPenbomb)    // FTP/HTTP credentials
	router.All("/.docker*", FiberPenbomb)  // .dockerenv, .docker/config.json

	// === Config & credential files ===
	router.All("/web.config", FiberPenbomb) // IIS config
	router.All("/*config.json", FiberPenbomb)
	router.All("/*config.yml", FiberPenbomb)
	router.All("/*config.yaml", FiberPenbomb)
	router.All("/*database.yml", FiberPenbomb)
	router.All("/*credentials*", FiberPenbomb)
	router.All("/*secrets*", FiberPenbomb)
	router.All("/id_rsa*", FiberPenbomb)
	router.All("/*aws_config*", FiberPenbomb)
	router.All("/*sftp-config.json", FiberPenbomb) // common editor credential leak

	// === Infrastructure / build files ===
	router.All("/Dockerfile*", FiberPenbomb)
	router.All("/docker-compose*", FiberPenbomb)
	router.All("/.dockerenv", FiberPenbomb)
	router.All("/Makefile", FiberPenbomb)
	router.All("/Jenkinsfile", FiberPenbomb)
	router.All("/.travis.yml", FiberPenbomb)
	router.All("/.circleci*", FiberPenbomb)
	router.All("/.github*", FiberPenbomb)
	router.All("/Vagrantfile", FiberPenbomb)
	router.All("/Procfile", FiberPenbomb) // Heroku
	router.All("/.terraform*", FiberPenbomb)
	router.All("/terraform*", FiberPenbomb)
	router.All("/ansible*", FiberPenbomb)
	router.All("/*.tfvars", FiberPenbomb)   // Terraform variable files
	router.All("/*.tfstate*", FiberPenbomb) // Terraform state (contains secrets)

	// === Package manifests & lockfiles ===
	router.All("/package.json", FiberPenbomb)
	router.All("/package-lock.json", FiberPenbomb)
	router.All("/yarn.lock", FiberPenbomb)
	router.All("/composer.json", FiberPenbomb)
	router.All("/composer.lock", FiberPenbomb)
	router.All("/Gemfile*", FiberPenbomb)
	router.All("/requirements.txt", FiberPenbomb)
	router.All("/Pipfile*", FiberPenbomb)
	router.All("/pom.xml", FiberPenbomb) // Maven
	router.All("/build.gradle*", FiberPenbomb)

	// === Log & backup files ===
	router.All("/*.log", FiberPenbomb)
	router.All("/*.bak", FiberPenbomb)
	router.All("/*.backup", FiberPenbomb)
	router.All("/*.old", FiberPenbomb)
	router.All("/*.orig", FiberPenbomb)
	router.All("/*.sql", FiberPenbomb)
	router.All("/*.dump", FiberPenbomb)
	router.All("/*.gz", FiberPenbomb)
	router.All("/*.zip", FiberPenbomb)
	router.All("/*.tar", FiberPenbomb)
	router.All("/*.swp", FiberPenbomb) // vim swap files
	router.All("/*.tmp", FiberPenbomb)

	// === Cryptographic key & certificate files ===
	router.All("/*.key", FiberPenbomb)
	router.All("/*.pem", FiberPenbomb)
	router.All("/*.p12", FiberPenbomb)
	router.All("/*.pfx", FiberPenbomb)
	router.All("/*.crt", FiberPenbomb)
	router.All("/*.cer", FiberPenbomb)
	router.All("/*.jks", FiberPenbomb) // Java KeyStore

	// === Server / framework enumeration ===
	router.All("/cgi-bin*", FiberPenbomb)
	router.All("/server-status", FiberPenbomb) // Apache mod_status
	router.All("/server-info", FiberPenbomb)   // Apache mod_info
	router.All("/phpinfo*", FiberPenbomb)
	router.All("/trace", FiberPenbomb)       // HTTP TRACE method probe (also registered for all methods)
	router.All("/*actuator*", FiberPenbomb)  // Spring Boot actuator endpoints
	router.All("/*_profiler*", FiberPenbomb) // Symfony profiler

	// === API documentation & schema (probed for sensitive endpoint discovery) ===
	router.All("/swagger*", FiberPenbomb)
	router.All("/openapi*", FiberPenbomb)
	router.All("/api-docs*", FiberPenbomb)
	router.All("/api/swagger*", FiberPenbomb)
	router.All("/graphql*", FiberPenbomb) // GraphQL introspection
	router.All("/graphiql*", FiberPenbomb)
	router.All("/altair*", FiberPenbomb) // GraphQL GUI

	// === Cloud provider metadata & credential endpoints ===
	router.All("/latest/meta-data*", FiberPenbomb) // AWS EC2 IMDSv1
	router.All("/metadata*", FiberPenbomb)         // GCP/Azure metadata
	router.All("/v1/auth*", FiberPenbomb)          // HashiCorp Vault
	router.All("/v1/secret*", FiberPenbomb)        // HashiCorp Vault
	router.All("/v1/keys*", FiberPenbomb)          // HashiCorp Vault

	// === IDE & editor artifacts ===
	router.All("/.vscode*", FiberPenbomb)
	router.All("/.idea*", FiberPenbomb) // JetBrains IDEs
	router.All("/.vite*", FiberPenbomb)
	router.All("/.fleet*", FiberPenbomb) // JetBrains Fleet
	router.All("/.eclipse*", FiberPenbomb)
}
