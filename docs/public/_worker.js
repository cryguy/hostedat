export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const path = url.pathname;

    // Intercept binary download requests and redirect to public object URLs.
    if (path.startsWith("/downloads/") && path !== "/downloads/" && !path.endsWith(".json")) {
      const filename = path.replace("/downloads/", "");

      // Only redirect for known binary patterns.
      if (/^hostedat(-server)?(-v8)?-(linux|darwin|windows)-(amd64|arm64|universal)(\.exe)?$/.test(filename)) {
        if (!env.DOWNLOADS) {
          return new Response("Storage not configured", { status: 503 });
        }

        // Check the file exists in the bucket.
        const head = await env.DOWNLOADS.head(filename);
        if (!head) {
          return new Response("File not found", { status: 404 });
        }

        const objectUrl = env.DOWNLOADS.publicUrl(filename);
        return Response.redirect(objectUrl, 302);
      }
    }

    // Intercept markdown reference docs and replace domain placeholders.
    if (path.endsWith(".md")) {
      const response = await env.ASSETS.fetch(request);
      if (!response.ok) return response;

      // Derive the base domain by stripping the first subdomain (e.g. docs.hostedat.example.com -> hostedat.example.com).
      const host = url.hostname;
      const parts = host.split(".");
      const domain = parts.length > 2 ? parts.slice(1).join(".") : host;

      let body = await response.text();
      // Replace storage.example.com first (more specific), then bare example.com.
      body = body.replaceAll("storage.example.com", "storage." + domain);
      body = body.replaceAll("example.com", domain);

      return new Response(body, {
        status: response.status,
        headers: {
          "Content-Type": "text/markdown; charset=utf-8",
          "Cache-Control": "public, max-age=300",
          "Access-Control-Allow-Origin": "*",
        },
      });
    }

    // Everything else: serve from static assets.
    return env.ASSETS.fetch(request);
  },
};
