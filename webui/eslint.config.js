import js from "@eslint/js";
import { defineConfig } from "eslint/config";
import tseslint from "typescript-eslint";

const stateModules = "**/state/**";
const apiModules = "**/api/**";
const featureModules = "**/features/**";
const uiModules = "**/ui/**";
const domainModules = "**/domain/**";
const elementLookups = "**/*-elements";
const typeModules = ["**/types", "**/types/**"];

function layerBoundary(files, forbidden, layerRule) {
  return {
    files,
    rules: {
      "no-restricted-imports": ["error", {patterns: [{group: forbidden, message: layerRule}]}]
    }
  };
}

export default defineConfig(
  {
    ignores: ["node_modules", "../internal/webui/assets", "eslint.config.js"]
  },
  js.configs.recommended,
  ...tseslint.configs.recommendedTypeChecked,
  {
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname
      }
    },
    rules: {
      "no-undef": "off",
      "@typescript-eslint/no-explicit-any": "error",
      "no-restricted-properties": ["error",
        { property: "innerHTML", message: "Render markup with setHTML() from lib/safe-html.ts." },
        { property: "outerHTML", message: "Render markup with setOuterHTML() from lib/safe-html.ts." },
        { property: "insertAdjacentHTML", message: "Render markup with setHTML() from lib/safe-html.ts." }
      ],
      "no-restricted-syntax": ["error",
        {
          selector: "TemplateLiteral:not(TaggedTemplateExpression > .quasi) > TemplateElement[value.raw=/<[a-zA-Z]/]",
          message: "Build markup with the html tag from lib/safe-html.ts so interpolations are escaped."
        },
        {
          selector: "Literal[value=/<[a-zA-Z][^>]*>/]",
          message: "Build markup with the html tag from lib/safe-html.ts so interpolations are escaped."
        }
      ]
    }
  },
  {
    files: ["src/lib/safe-html.ts", "src/test/**"],
    rules: {
      "no-restricted-properties": "off",
      "no-restricted-syntax": "off"
    }
  },
  layerBoundary(["src/lib/**"], [stateModules, apiModules, featureModules, uiModules, domainModules, ...typeModules], "lib/ holds app-agnostic utilities only."),
  layerBoundary(["src/domain/**"], [stateModules, apiModules, featureModules, uiModules], "domain/ holds pure shared app logic."),
  layerBoundary(["src/ui/**"], [stateModules, apiModules, featureModules, domainModules], "ui/ components take data through parameters."),
  layerBoundary(["src/api/**"], [stateModules, featureModules, uiModules], "api/ talks to the server only."),
  layerBoundary(["src/features/**/*-data.ts"], [stateModules, apiModules, uiModules, elementLookups], "*-data.ts modules compute from their parameters only."),
  layerBoundary(["src/features/**/*-view.ts"], [stateModules, apiModules, elementLookups], "*-view.ts modules build markup from their parameters only.")
);
