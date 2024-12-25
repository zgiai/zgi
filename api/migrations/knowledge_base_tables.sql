-- MySQL dump 10.13  Distrib 9.0.1, for macos15.1 (arm64)
--
-- Host: 127.0.0.1    Database: zgi
-- ------------------------------------------------------
-- Server version	9.0.1

/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!50503 SET NAMES utf8mb4 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;

--
-- Table structure for table `knowledge_bases`
--

DROP TABLE IF EXISTS `knowledge_bases`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `knowledge_bases` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `description` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci,
  `visibility` enum('PRIVATE','PUBLIC','ORGANIZATION') COLLATE utf8mb4_general_ci DEFAULT 'PRIVATE',
  `owner_id` int NOT NULL,
  `organization_id` int DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `status` int NOT NULL DEFAULT '1' COMMENT 'Knowledge base status: 1=ACTIVE, 0=DISABLED, -1=DELETED',
  `collection_name` varchar(255) COLLATE utf8mb4_general_ci NOT NULL,
  `model` varchar(255) COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'text-embedding-3-small',
  `dimension` int NOT NULL DEFAULT '1536',
  `document_count` int DEFAULT '0',
  `total_chunks` int DEFAULT '0',
  `total_tokens` int DEFAULT '0',
  `meta_info` json DEFAULT NULL,
  `tags` json DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `collection_name` (`collection_name`),
  KEY `owner_id` (`owner_id`),
  KEY `ix_knowledge_bases_id` (`id`),
  KEY `idx_org` (`organization_id`),
  KEY `idx_visibility` (`visibility`),
  CONSTRAINT `knowledge_bases_ibfk_1` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`),
  CONSTRAINT `knowledge_bases_ibfk_2` FOREIGN KEY (`organization_id`) REFERENCES `organizations` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=14 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `knowledge_documents`
--

DROP TABLE IF EXISTS `knowledge_documents`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `knowledge_documents` (
  `id` int NOT NULL AUTO_INCREMENT,
  `kb_id` int NOT NULL,
  `file_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `file_path` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL,
  `file_type` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `file_size` int NOT NULL,
  `file_hash` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` int DEFAULT NULL,
  `error_message` text COLLATE utf8mb4_unicode_ci,
  `chunk_count` int DEFAULT NULL,
  `token_count` int DEFAULT NULL,
  `vector_count` int DEFAULT NULL,
  `title` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `language` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `author` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `source_url` varchar(512) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `meta_info` json DEFAULT NULL,
  `tags` json DEFAULT NULL,
  `chunk_size` int DEFAULT NULL,
  `chunk_overlap` int DEFAULT NULL,
  `embedding_model` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `processed_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `ix_knowledge_documents_id` (`id`),
  KEY `idx_kb_status` (`kb_id`,`status`),
  KEY `idx_file_hash` (`file_hash`),
  CONSTRAINT `knowledge_documents_ibfk_1` FOREIGN KEY (`kb_id`) REFERENCES `knowledge_bases` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `knowledge_document_chunks`
--

DROP TABLE IF EXISTS `knowledge_document_chunks`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `knowledge_document_chunks` (
  `id` int NOT NULL AUTO_INCREMENT,
  `document_id` int NOT NULL,
  `chunk_index` int NOT NULL,
  `content` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `token_count` int NOT NULL,
  `vector_id` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `embedding_model` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `meta_info` json DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `ix_knowledge_document_chunks_id` (`id`),
  KEY `idx_document_chunk` (`document_id`,`chunk_index`),
  KEY `idx_vector_id` (`vector_id`),
  CONSTRAINT `knowledge_document_chunks_ibfk_1` FOREIGN KEY (`document_id`) REFERENCES `knowledge_documents` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;

/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;
/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

-- Dump completed on 2024-12-25  0:23:10
