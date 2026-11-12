import { strict as assert } from "node:assert";
import { test } from "node:test";

import { renderDesignV3 } from "./design-v3";

// Only the guards that reject BEFORE any S3 or CLI work. Everything past them
// needs a bucket and a real Hyperframes install, which is an integration
// concern, not a unit one.

test("a job without ids is refused before any work starts", async () => {
    await assert.rejects(
        () => renderDesignV3({ brandId: "", designId: "d1", projectPrefix: "p/" }),
        /brandId and designId are required/
    );
    await assert.rejects(
        () => renderDesignV3({ brandId: "b1", designId: "", projectPrefix: "p/" }),
        /brandId and designId are required/
    );
});

test("a design with no project is refused with a reason, not a crash", async () => {
    // This is the state a design sits in between being created and the
    // director writing its files. Rendering it is a user-visible no-op, so it
    // has to say so rather than fail inside the S3 client.
    await assert.rejects(
        () => renderDesignV3({ brandId: "b1", designId: "d1", projectPrefix: "" }),
        /no project to render/
    );
});
