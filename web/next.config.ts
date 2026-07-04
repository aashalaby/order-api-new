import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Standalone output -> a self-contained server.js tree we copy into a
  // minimal node image and run as a second Scaleway Serverless Container
  // (see web/Dockerfile and the ci.yaml docker/deploy matrix).
  output: "standalone",
};

export default nextConfig;
