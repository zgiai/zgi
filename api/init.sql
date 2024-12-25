-- Create database
CREATE DATABASE IF NOT EXISTS zgi;
USE zgi;

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;


-- Create tables for knowledge base functionality
CREATE TABLE IF NOT EXISTS `knowledge_bases` (
    `id` int(11) NOT NULL AUTO_INCREMENT,
    `name` varchar(255) NOT NULL,
    `description` text,
    `visibility` enum('private','public','organization') DEFAULT 'private',
    `status` int(11) DEFAULT 1,
    `collection_name` varchar(255) NOT NULL UNIQUE,
    `model` varchar(255) NOT NULL DEFAULT 'text-embedding-3-small',
    `dimension` int(11) NOT NULL DEFAULT 1536,
    `document_count` int(11) DEFAULT 0,
    `total_chunks` int(11) DEFAULT 0,
    `total_tokens` int(11) DEFAULT 0,
    `metadata` json,
    `tags` json,
    `owner_id` int(11) NOT NULL,
    `organization_id` int(11),
    `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
    `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_owner_status` (`owner_id`, `status`),
    FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`),
    FOREIGN KEY (`organization_id`) REFERENCES `organizations` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `documents` (
    `id` int(11) NOT NULL AUTO_INCREMENT,
    `kb_id` int(11) NOT NULL,
    `file_name` varchar(255) NOT NULL,
    `file_path` varchar(255),
    `file_type` varchar(50) NOT NULL,
    `status` int(11) DEFAULT 1,
    `chunk_count` int(11) DEFAULT 0,
    `metadata` json,
    `error_message` text,
    `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
    `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_kb_status` (`kb_id`, `status`),
    FOREIGN KEY (`kb_id`) REFERENCES `knowledge_bases` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `upload_file` (
    `id` int(11) NOT NULL AUTO_INCREMENT,
  `local` varchar(255) NOT NULL,
  `cdn_path` varchar(255) DEFAULT NULL,
  `create_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

SET FOREIGN_KEY_CHECKS = 1;