## {#welcome} Willkommen bei Voxis Source-Available

Voxis Source-Available macht aus Audio durchsuchbare Transkripte und KI-Zusammenfassungen. Sie können Ihre Ergebnisse auch exportieren und andere Werkzeuge über die API oder MCP anbinden.

Ihre Organisation betreibt Voxis auf einem eigenen Server. Für ein Transkript sendet der Server Ihr Audio an Speechmatics, einen Cloud-Transkriptionsdienst, und nutzt dabei das Speechmatics-Konto Ihrer Organisation. Das Audio wird unter einem neutralen Dateinamen gesendet, nicht unter Ihrem ursprünglichen Dateinamen. Danach bittet Voxis Speechmatics, den Auftrag zu löschen.

Zusammenfassungen schreibt ein Gemma-KI-Modell, das Ihre Organisation selbst betreibt. Ihr Transkript geht nur an dieses Modell.

Voxis verschlüsselt gespeicherte Audiodateien, Transkripte und Zusammenfassungen. Dateiangaben wie Namen und Titel sowie Kontodaten werden ohne diese Verschlüsselung gespeichert. Wer den Server betreibt, kann auf Ihre Daten zugreifen. Nutzen Sie Voxis daher nur, wenn Sie diesen Personen vertrauen.

## {#sign-in} Anmeldung und Kontosicherheit

Ihre Installation verwendet den gebündelten Keycloak-Realm `voxis-oss` und den Client `voxis-oss-web`. Die Anmeldung nutzt PKCE mit S256 und benötigt daher HTTPS oder localhost.

- Bitten Sie Ihren Administrator, Ihr Konto anzulegen und ihm Zugriff auf Voxis zu geben.
- Wenn Sie sich anmelden können, aber eine Meldung sehen, dass Sie sich an Ihren Administrator wenden sollen, hat Ihr Konto noch keinen Zugriff. Ihr Administrator kann das in Keycloak ändern.
- Ändern Sie Passwort und Mehrfaktor-Authentifizierung in der Keycloak-Kontokonsole.
- Voxis Source-Available bietet keine Selbstregistrierung, kein Social Login, keine E-Mail-Verifizierung und keine Kontolöschung. Wenden Sie sich an Ihren Administrator.

## {#upload} Audio hochladen

Wählen Sie Hochladen im Dashboard oder in der Seitenleiste. Wählen Sie eine Audiodatei, warten Sie auf Scan und verschlüsselte Speicherung und starten Sie dann die Transkription im Dateileser.

In der Bibliothek können Sie Dateidetails ändern, den Leser öffnen, Audio und Transkripte durchsuchen und eigene Medien löschen. Bei der Suche im Transkriptinhalt prüft Voxis abgeschlossene Transkripte seitenweise in begrenzten Schritten. Verwenden Sie **Weitere Transkripttreffer laden**, wenn die Schaltfläche erscheint, um mit älteren abgeschlossenen Transkripten fortzufahren.

Das Löschen einer Datei ist endgültig. Audio, Transkripte, Zusammenfassungen sowie Name, Titel und Beschreibung der Datei werden vom Server entfernt. Das lässt sich nicht rückgängig machen. Kopien in bereits vorhandenen Sicherungen Ihrer Organisation bleiben erhalten, bis diese Sicherungen gelöscht werden.

Benutzerdefinierte Wörterbücher und Vokabelpakete sind mit Speechmatics Melia 1 nicht verfügbar.

[[screenshot:dashboard-capture|Voxis-Dashboard mit Optionen zum Hochladen und Aufzeichnen]]

[[screenshot:library-search|Voxis-Bibliothekssuche mit Treffern im Transkriptinhalt und Fortsetzen-Steuerung]]

## {#record} Aufnehmen und wiederherstellen

Wählen Sie Aufnehmen, um Mikrofon- oder unterstütztes Geräteaudio im Browser aufzunehmen. Wenn die Seite geschlossen wird oder das Netzwerk ausfällt, bietet der Rekorder bei Ihrer Rückkehr an, die unterbrochene Sitzung wiederherzustellen.

Bis zum Upload bleiben Teile Ihrer Aufnahme in diesem Browser gespeichert. Sie sind verschlüsselt, aber der Schlüssel liegt im selben Browser. Das schützt also nur vor einem flüchtigen Blick. Beim Abmelden löscht Voxis diese Teile. Wenn noch nicht alle Teile hochgeladen sind, warnt Voxis Sie vorher. Die Teile werden auch gelöscht, wenn sich eine andere Person in diesem Browser anmeldet. Warten Sie an einem gemeinsam genutzten Computer, bis der Upload fertig ist, und melden Sie sich dann ab.

Wenn Ihr Administrator eine Aufbewahrungsfrist für Aufnahmen festgelegt hat, wird das Audio von Browser-Aufnahmen nach Ablauf dieser Frist endgültig gelöscht. Transkript und Zusammenfassungen bleiben erhalten.

Standardaufnahmen werden unterstützt. Privilegierte Aufnahmen sind nicht Teil von Voxis Source-Available.

Wenn eine Transkription fehlschlägt, zeigt ihre Aktivitätskarte eine sichere Fehlerkategorie. Öffnen Sie den Eintrag, prüfen Sie bei Bedarf die Anbieter-Einstellung und versuchen Sie es erneut.

Das Bild unten ist ein kontrolliertes Beispiel mit synthetischen Daten. Es zeigt die Fehlerkarte, nicht ein Live-Ereignis des Dienstes.

[[screenshot:activity-failure|Kontrollierte Voxis-Aktivitätskarte mit einer sicheren Meldung zu einer fehlgeschlagenen Transkription]]

[[screenshot:record|Voxis-Aufzeichnungsseite im Browser]]

## {#summaries} Zusammenfassungen lesen und exportieren

Der Leser zeigt Transkripte, Sprecheränderungen und die verfügbaren professionellen Zusammenfassungsformate. Das lokale Gemma-Modell erstellt Zusammenfassungen aus dem konfigurierten öffentlichen Prompt-Satz. Profile für anspruchsvolle Zusammenfassungen sind ausgeschaltet, bis Ihr Administrator sie aktiviert.

Export ist als PDF, DOCX und JSON verfügbar. Der PDF-Export unterstützt chinesische, japanische und koreanische Schrift nicht zuverlässig; verwenden Sie dafür DOCX oder JSON. Der BAP-Export bleibt Opt-in und ist aus, bis der Betreiber ihn aktiviert.

Um eine Kopie Ihrer Arbeit zu behalten, laden Sie das Audio herunter und exportieren Sie jedes Transkript und jede Zusammenfassung. Für viele Einträge können Sie auch einen API-Schlüssel oder MCP-Tools verwenden.

[[screenshot:reader|Voxis-Transkriptleser]]

## {#admin} Administration, API und MCP

Administratoren können den Betriebsstatus prüfen, die Aufbewahrung von Aufnahmen festlegen und die öffentlichen Gemma-Präsentationsprompts bearbeiten. Modellname, Laufzeit, Revision, Quantisierung und verfügbare Nutzungsdaten erscheinen im Administrationsbereich, wenn der Server sie meldet. Speicherkontingente werden in dieser Edition nicht durchgesetzt.

API-Schlüssel und die MCP-Tools folgen denselben Organisations- und Eigentumsregeln wie die Anwendung. Halten Sie Schlüssel aus Browser-Code heraus und teilen Sie sie nur mit Tools, denen Sie vertrauen. Wird der Inhaber eines Schlüssels deaktiviert oder verliert er den Zugriff, funktioniert der Schlüssel nach etwa einer Minute nicht mehr.

[[screenshot:briefings|Voxis-Ansicht für Zusammenfassung und Export]]

Speechmatics bietet auch eine On-Premises-Bereitstellung für Organisationen an, die Audio in ihrer eigenen Infrastruktur verarbeiten möchten. Wenden Sie sich für separat vereinbarte Lizenzierung und Bereitstellung an Speechmatics. Sie ist für diese Version nicht Ende zu Ende verifiziert.
