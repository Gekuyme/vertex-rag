import { createReadStream } from "node:fs";
import { writeFile } from "node:fs/promises";
import { test, expect } from "@playwright/test";

test("login -> upload -> strict chat with citations", async ({ request }, testInfo) => {
  const timestamp = Date.now();
  const email = `pw_e2e_${timestamp}@example.com`;
  const password = "Password123!";
  const organizationName = `Playwright E2E ${timestamp}`;
  const tokenKey = `playwright_token_${timestamp}`;

  const registerResponse = await request.post("/auth/register_owner", {
    data: {
      organization_name: organizationName,
      email,
      password
    }
  });
  expect(registerResponse.ok()).toBeTruthy();
  const registerPayload = await registerResponse.json();
  const accessToken = registerPayload.access_token as string;
  const ownerRoleID = registerPayload.user.role_id as number;

  const documentPath = testInfo.outputPath("playwright_doc.txt");
  await writeFile(
    documentPath,
    `Playwright E2E KB document.\nToken: ${tokenKey}\nThis should appear in strict citations.\n`,
    "utf-8"
  );

  const uploadResponse = await request.post("/documents/upload", {
    headers: {
      Authorization: `Bearer ${accessToken}`
    },
    multipart: {
      title: "Playwright E2E Document",
      allowed_role_ids: String(ownerRoleID),
      file: createReadStream(documentPath)
    }
  });
  expect(uploadResponse.ok()).toBeTruthy();
  const uploadPayload = await uploadResponse.json();
  const documentID = uploadPayload.id as string;

  let status = "uploaded";
  for (let index = 0; index < 60; index++) {
    const listResponse = await request.get("/documents", {
      headers: {
        Authorization: `Bearer ${accessToken}`
      }
    });
    expect(listResponse.ok()).toBeTruthy();
    const listPayload = await listResponse.json();
    const entry = (listPayload.documents as Array<{ id: string; status: string }>).find(
      (item) => item.id === documentID
    );
    if (entry) {
      status = entry.status;
    }
    if (status === "ready") {
      break;
    }
    await new Promise((resolve) => setTimeout(resolve, 2000));
  }
  expect(status).toBe("ready");

  const createChatResponse = await request.post("/chats", {
    headers: {
      Authorization: `Bearer ${accessToken}`
    },
    data: {
      title: "Playwright E2E Chat"
    }
  });
  expect(createChatResponse.ok()).toBeTruthy();
  const createChatPayload = await createChatResponse.json();
  const chatID = createChatPayload.id as string;

  const messageResponse = await request.post(`/chats/${chatID}/messages`, {
    headers: {
      Authorization: `Bearer ${accessToken}`
    },
    data: {
      content: tokenKey,
      mode: "strict",
      top_k: 8,
      candidate_k: 32
    }
  });
  expect(messageResponse.ok()).toBeTruthy();
  const messagePayload = await messageResponse.json();

  expect(messagePayload.mode).toBe("strict");
  expect(Array.isArray(messagePayload.citations)).toBeTruthy();
  expect((messagePayload.citations as unknown[]).length).toBeGreaterThan(0);
});
