import type { SecurityTarget } from "../../types";

export type SecurityFormSubmitter = (values: FormData, target: SecurityTarget) => Promise<void>;
