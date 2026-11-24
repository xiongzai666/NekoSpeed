package com.nekospeed.android

import com.nekospeed.android.service.ReadyEndpoint
import com.nekospeed.android.service.ServerUrlParser
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class AndroidShellLogicTest {

    @get:Rule
    val tempFolder = TemporaryFolder()

    @Test
    fun parseReadyUrl_matchesPrimaryMarker() {
        val parsed = ServerUrlParser.parseReadyUrl("NEKOSPEED_WEB_READY=http://127.0.0.1:41235")
        assertEquals(ReadyEndpoint("http://127.0.0.1:41235", 41235), parsed)
    }

    @Test
    fun parseReadyUrl_matchesFallbackUrlInLogLine() {
        val parsed = ServerUrlParser.parseReadyUrl(
            "Web UI 已启动：http://127.0.0.1:18189 （已启用令牌鉴权，支持 Header 或 ?token=...）"
        )
        assertEquals(ReadyEndpoint("http://127.0.0.1:18189", 18189), parsed)
    }

    @Test
    fun parseReadyUrl_rejectsInvalidPortsAndNonLoopbackHosts() {
        assertNull(ServerUrlParser.parseReadyUrl("NEKOSPEED_WEB_READY=http://127.0.0.1:0"))
        assertNull(ServerUrlParser.parseReadyUrl("NEKOSPEED_WEB_READY=http://127.0.0.1:70000"))
        assertNull(ServerUrlParser.parseReadyUrl("NEKOSPEED_WEB_READY=http://0.0.0.0:18189"))
        assertNull(ServerUrlParser.parseReadyUrl("http://192.168.1.10:18189"))
        assertNull(ServerUrlParser.parseReadyUrl("starting geoip check..."))
    }

    @Test
    fun isSafeSegment_validatesNames() {
        assertTrue(ServerUrlParser.isSafeSegment("20261124_153000"))
        assertTrue(ServerUrlParser.isSafeSegment("20261124_153000_result.json"))
        assertTrue(ServerUrlParser.isSafeSegment("机场测速_blue_dark.png"))

        assertFalse(ServerUrlParser.isSafeSegment(""))
        assertFalse(ServerUrlParser.isSafeSegment("."))
        assertFalse(ServerUrlParser.isSafeSegment(".."))
        assertFalse(ServerUrlParser.isSafeSegment(".hidden"))
        assertFalse(ServerUrlParser.isSafeSegment("a/b"))
        assertFalse(ServerUrlParser.isSafeSegment("a\\b"))
        assertFalse(ServerUrlParser.isSafeSegment("../secret"))
        assertFalse(ServerUrlParser.isSafeSegment("bad\u0000name"))
        assertFalse(ServerUrlParser.isSafeSegment("bad\nname"))
        assertFalse(ServerUrlParser.isSafeSegment("C:evil.txt"))
    }

    @Test
    fun resolveSafeWebFile_allowsContainmentAndBlocksTraversal() {
        val baseDir = tempFolder.newFolder("files")
        val webRunDir = File(baseDir, "out/web/20261124_153000").apply { mkdirs() }
        val expectedFile = File(webRunDir, "report.png").apply { writeText("png") }

        val resolved = ServerUrlParser.resolveSafeWebFile(baseDir, "20261124_153000", "report.png")
        assertNotNull(resolved)
        assertEquals(expectedFile.canonicalFile, resolved)

        assertNull(ServerUrlParser.resolveSafeWebFile(baseDir, "..", "report.png"))
        assertNull(ServerUrlParser.resolveSafeWebFile(baseDir, "../20261124_153000", "report.png"))
        assertNull(ServerUrlParser.resolveSafeWebFile(baseDir, "20261124_153000", "../secret.txt"))
        assertNull(ServerUrlParser.resolveSafeWebFile(baseDir, "20261124_153000", "sub/report.png"))
        assertNull(ServerUrlParser.resolveSafeWebFile(baseDir, "", "report.png"))
        assertNull(ServerUrlParser.resolveSafeWebFile(baseDir, "20261124_153000", ".env"))
    }

    @Test
    fun resolveSafeExportFile_sanitizesIntoExportsDir() {
        val baseDir = tempFolder.newFolder("export_files")
        val exportsRoot = File(baseDir, "out/exports").canonicalFile

        val normal = ServerUrlParser.resolveSafeExportFile(baseDir, "nekospeed_20261124.csv")
        assertNotNull(normal)
        assertEquals(exportsRoot, normal!!.parentFile)
        assertEquals("nekospeed_20261124.csv", normal.name)

        val traversed = ServerUrlParser.resolveSafeExportFile(baseDir, "../../etc/passwd")
        assertNotNull(traversed)
        assertEquals(exportsRoot, traversed!!.parentFile)
        assertEquals("passwd.csv", traversed.name)

        val fallback = ServerUrlParser.resolveSafeExportFile(baseDir, "...")
        assertNotNull(fallback)
        assertEquals(exportsRoot, fallback!!.parentFile)
        assertEquals("nekospeed_export.csv", fallback.name)
    }
}
