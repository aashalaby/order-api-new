// ESLint 9 flat config, the Next.js 16 way: eslint-config-next now ships
// native flat configs, imported directly. Do NOT reintroduce FlatCompat
// here — routing these presets through the eslintrc compat layer crashes
// with "Converting circular structure to JSON" because the translated
// plugin objects are self-referencing.
import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  globalIgnores([".next/**", "out/**", "build/**", "node_modules/**", "next-env.d.ts"]),
]);

export default eslintConfig;
