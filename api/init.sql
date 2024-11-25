CREATE DATABASE IF NOT EXISTS zgi;
USE zgi;

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

CREATE TABLE IF NOT EXISTS `upload_file` (
    `id` int(11) NOT NULL AUTO_INCREMENT,
  `local` varchar(255) NOT NULL,
  `cdn_path` varchar(255) DEFAULT NULL,
  `create_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;