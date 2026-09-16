package connectorfinalize

import (
	"slices"

	"gorm.io/gorm"
)

// Walk PostgreSQL's dependency graph, including relation row types, views,
// rules, owned sequences and expression dependencies. A text search alone
// misses parsed SQL references and schema-qualified references.
func retirementDependencies(db *gorm.DB) ([]row, error) {
	rows, err := db.Raw(`WITH RECURSIVE roots(classid,objid,objsubid) AS (
 SELECT 'pg_class'::regclass::oid,c.oid,0 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relname IN ?
 UNION
 SELECT 'pg_class'::regclass::oid,c.oid,a.attnum::int FROM pg_class c
 JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid
 WHERE n.nspname='public' AND c.relname='connector_target_bindings' AND a.attname IN ('compatibility','compatibility_state') AND NOT a.attisdropped
), objects(classid,objid,objsubid) AS (
 SELECT * FROM roots
 UNION
 SELECT d.classid,d.objid,d.objsubid FROM pg_depend d JOIN objects o
 ON d.refclassid=o.classid AND d.refobjid=o.objid AND (o.objsubid=0 OR d.refobjsubid=o.objsubid)
 WHERE d.deptype IN ('n','a','i','P','S')
), owned AS (
 SELECT o.*, CASE
 WHEN o.classid='pg_class'::regclass THEN COALESCE(
   (SELECT i.indrelid FROM pg_index i WHERE i.indexrelid=o.objid),
   (SELECT d.refobjid FROM pg_depend d WHERE d.classid=o.classid AND d.objid=o.objid AND d.refclassid='pg_class'::regclass AND d.deptype IN ('a','i') LIMIT 1),o.objid)
 WHEN o.classid='pg_constraint'::regclass THEN (SELECT conrelid FROM pg_constraint WHERE oid=o.objid)
 WHEN o.classid='pg_trigger'::regclass THEN (SELECT CASE WHEN t.tgisinternal AND k.oid IS NOT NULL THEN k.conrelid ELSE t.tgrelid END FROM pg_trigger t LEFT JOIN pg_constraint k ON k.oid=t.tgconstraint WHERE t.oid=o.objid)
 WHEN o.classid='pg_rewrite'::regclass THEN (SELECT ev_class FROM pg_rewrite WHERE oid=o.objid)
 WHEN o.classid='pg_attrdef'::regclass THEN (SELECT adrelid FROM pg_attrdef WHERE oid=o.objid)
 WHEN o.classid='pg_policy'::regclass THEN (SELECT polrelid FROM pg_policy WHERE oid=o.objid)
 WHEN o.classid='pg_type'::regclass THEN (SELECT CASE WHEN t.typrelid<>0 THEN t.typrelid ELSE e.typrelid END FROM pg_type t LEFT JOIN pg_type e ON e.oid=t.typelem WHERE t.oid=o.objid)
 ELSE 0 END AS owner_oid
 FROM objects o
)
SELECT jsonb_build_object('kind','dependency','class',o.classid::regclass::text,
 'identity',pg_describe_object(o.classid,o.objid,o.objsubid),
 'owner_table',COALESCE(toast_owner.relname,relation.relname,''),
 'owner_schema',COALESCE(toast_namespace.nspname,relation_namespace.nspname,''),
 'constraint_name',COALESCE(k.conname,''),
 'attribute_name',COALESCE(a.attname,''),
 'root',EXISTS(SELECT 1 FROM roots r WHERE r.classid=o.classid AND r.objid=o.objid AND r.objsubid=o.objsubid))
FROM owned o LEFT JOIN pg_class relation ON relation.oid=o.owner_oid
LEFT JOIN pg_namespace relation_namespace ON relation_namespace.oid=relation.relnamespace
LEFT JOIN pg_class toast_owner ON toast_owner.reltoastrelid=relation.oid
LEFT JOIN pg_namespace toast_namespace ON toast_namespace.oid=toast_owner.relnamespace
LEFT JOIN pg_constraint k ON o.classid='pg_constraint'::regclass AND k.oid=o.objid
LEFT JOIN pg_attrdef ad ON o.classid='pg_attrdef'::regclass AND ad.oid=o.objid
LEFT JOIN pg_attribute a ON a.attrelid=ad.adrelid AND a.attnum=ad.adnum`, RetiredTables()).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	size := 0
	return readRows(rows, &size)
}

func validateRetirementDependencies(objects []row, report *Report) {
	for _, object := range objects {
		if root, _ := object["root"].(bool); root {
			continue
		}
		owner := text(object, "owner_table")
		if text(object, "owner_schema") == "public" && slices.Contains(RetiredTables(), owner) {
			continue
		}
		if text(object, "owner_schema") == "public" && owner == "connector_target_bindings" && text(object, "class") == "pg_constraint" &&
			slices.Contains([]string{"connector_target_bindings_compatibility_check", "connector_target_bindings_compatibility_object_check"}, text(object, "constraint_name")) {
			continue
		}
		if text(object, "owner_schema") == "public" && owner == "connector_target_bindings" && text(object, "class") == "pg_attrdef" &&
			slices.Contains([]string{"compatibility", "compatibility_state"}, text(object, "attribute_name")) {
			continue
		}
		// Deliberately withhold SQL/object descriptions from a portable report;
		// these can contain custom expressions or private object names.
		addIssue(report, "retirement_schema", "unexpected_dependency")
	}
}
