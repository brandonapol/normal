import { writeFileSync } from "node:fs";
import { BASELINE_CONFIG } from "@normal/schema";

writeFileSync("examples/baseline.config.json", `${JSON.stringify(BASELINE_CONFIG, null, 2)}\n`);
