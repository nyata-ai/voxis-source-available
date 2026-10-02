## {#welcome} Welcome to Voxis Source-Available

Voxis Source-Available turns audio into searchable transcripts and AI summaries. You can also export your work and connect other tools through the API or MCP.

Your organization runs Voxis on its own server. To make a transcript, the server sends your audio to Speechmatics, a cloud transcription service, using your organization's Speechmatics account. It sends the audio under a generic filename, not your original filename. Voxis then asks Speechmatics to delete its copy of the job.

Summaries are written by a Gemma AI model that your organization runs itself. Your transcript is sent only to that model.

Voxis encrypts the audio, transcripts, and summaries it stores. File details such as names and titles, and account details, are stored without that encryption. The people who run the server can reach your data, so use Voxis only if you trust them.

## {#sign-in} Sign in and account security

Your installation uses the bundled Keycloak realm `voxis-oss` and the `voxis-oss-web` client. Sign-in needs HTTPS or localhost, because it uses PKCE with S256.

- Ask your administrator to create your account and give it access to Voxis.
- If you can sign in but see a message asking you to contact your administrator, your account does not have access yet. Your administrator can fix this in Keycloak.
- Change your password and multi-factor authentication in the Keycloak account console.
- There is no self-service sign-up, social login, email verification, or account deletion in Voxis Source-Available. Ask your administrator.

## {#upload} Upload audio

Choose Upload from the dashboard or sidebar. Select an audio file, wait for its scan and encrypted storage to finish, then start transcription from the file reader.

In the library you can edit file details, open the reader, search audio and transcripts, and delete media you own. When searching transcript content, Voxis checks completed transcripts in bounded pages. Use **Load more transcript matches** when it appears to continue through older completed transcripts.

Deleting a file is permanent. It removes the audio, its transcripts and summaries, and its name, title, and description from the server. It cannot be undone. Copies in your organization's existing backups remain until those backups are deleted.

Custom dictionaries and vocabulary packs are not available with Speechmatics Melia 1.

[[screenshot:dashboard-capture|Voxis dashboard with the upload and record choices]]

[[screenshot:library-search|A Voxis library search with transcript-content matches and the continuation control]]

## {#record} Record and recover

Choose Record to capture microphone or supported device audio in the browser. If the page closes or the network fails, the recorder offers to recover the interrupted session when you return.

Until they are uploaded, pieces of your recording are kept in this browser. They are encrypted, but the key is kept in the same browser, so this only stops casual snooping. When you sign out, Voxis clears them. If some pieces have not been uploaded yet, it warns you first. They are also cleared when someone else signs in on this browser. On a shared computer, let the upload finish and then sign out.

If your administrator has set recording retention, the audio of browser recordings is deleted permanently after that period. The transcript and summaries stay.

Standard recording is supported. Privileged recording is not part of Voxis Source-Available.

If a transcription fails, its activity card shows a safe failure category. Open the item, check the provider setting when appropriate, and retry.

The image below is a controlled example with synthetic data. It illustrates the failure card, not a live service event.

[[screenshot:activity-failure|A controlled Voxis activity card showing a safe transcription failure message]]

[[screenshot:record|Voxis browser recording screen]]

## {#summaries} Read summaries and export

The reader shows transcripts, speaker edits, and the available professional summary formats. The local Gemma model writes summaries from the configured public prompt set. High-stakes summary profiles are off unless your administrator turns them on.

Export is available in PDF, DOCX, and JSON. PDF export does not reliably support Chinese, Japanese, or Korean scripts; use DOCX or JSON for those scripts. The BAP export stays opt-in and is off unless the operator enables it.

To keep a copy of your own work, download the audio and export each transcript and summary. You can also use an API key or MCP tools to fetch many items.

[[screenshot:reader|Voxis transcript reader]]

## {#admin} Administration, API, and MCP

Administrators can check operational status, set recording retention, and edit the public Gemma presentation prompts. The served model name, runtime, revision, quantization, and available usage data appear in the administration area when the server reports them. Storage quotas are not enforced in this edition.

API keys and the MCP tools follow the same organization and ownership rules as the app. Keep keys out of browser code and share them only with tools you trust. A key stops working about a minute after its owner is disabled or loses access.

[[screenshot:briefings|Voxis summary and export view]]

Speechmatics also offers an on-premises deployment for organizations that want audio processed within their own infrastructure. Contact Speechmatics for separately arranged licensing and deployment. It is not verified end to end for this release.
