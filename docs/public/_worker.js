export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const path = url.pathname;

    // Intercept binary download requests and redirect to public object URLs.
    if (path.startsWith("/downloads/") && path !== "/downloads/" && !path.endsWith(".json")) {
      const filename = path.replace("/downloads/", "");

      // Only redirect for known binary patterns.
      if (/^hostedat(-server)?-(linux|darwin|windows)-(amd64|arm64)(\.exe)?$/.test(filename)) {
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

    // Everything else: serve from static assets.
    return env.ASSETS.fetch(request);
  },
};
