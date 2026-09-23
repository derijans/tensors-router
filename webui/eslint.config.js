import js from "@eslint/js";
import tseslint from "typescript-eslint";

export default tseslint.config(
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
        { property: "innerHTML", message: "Render markup with setHTML() from safe-html.ts." },
        { property: "outerHTML", message: "Render markup with setOuterHTML() from safe-html.ts." },
        { property: "insertAdjacentHTML", message: "Render markup with setHTML() from safe-html.ts." }
      ],
      "no-restricted-syntax": ["error",
        {
          selector: "TemplateLiteral:not(TaggedTemplateExpression > .quasi) > TemplateElement[value.raw=/<[a-zA-Z]/]",
          message: "Build markup with the html tag from safe-html.ts so interpolations are escaped."
        },
        {
          selector: "Literal[value=/<[a-zA-Z][^>]*>/]",
          message: "Build markup with the html tag from safe-html.ts so interpolations are escaped."
        }
      ]
    }
  },
  {
    files: ["src/safe-html.ts", "src/test/**"],
    rules: {
      "no-restricted-properties": "off",
      "no-restricted-syntax": "off"
    }
  }
);
