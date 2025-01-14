package qry

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/t2wu/qry/datatype"
	"github.com/t2wu/qry/mdl"

	"github.com/jinzhu/gorm"
)

// Remove strategy
// If pegassoc, record it as dissociate and you're done.
// If pegged, record it as to be removed., traverse into the struct. If then encounter a peg, record it
// to be removed and need to traverse into it further, if encounter a pegassoc, record it as to be
// dissociated.
// If no pegassoc and pegged under this struct, return.

type modelAndIds struct {
	modelObj mdl.IModel
	ids      []interface{} // to send to Gorm need to be interface not *datatype.UUID
}

type cargo struct {
	toProcess map[string]modelAndIds
}

func DeleteModelFixManyToManyAndPegAndPegAssoc(db *gorm.DB, modelObj mdl.IModel) error {
	if err := removeManyToManyAssociationTableElem(db, modelObj); err != nil {
		return err
	}

	// Delete nested field
	// Not yet support two-level of nested field
	v := reflect.Indirect(reflect.ValueOf(modelObj))

	peg := make(map[string]modelAndIds) // key: name of the table to be deleted, val: list of ids
	car := cargo{toProcess: peg}

	if err := markForDelete(db, v, car); err != nil {
		return err
	}

	// Now actually delete
	for tblName := range car.toProcess {
		if err := db.Unscoped().Delete(car.toProcess[tblName].modelObj, car.toProcess[tblName].ids).Error; err != nil {
			return err
		}
	}

	return nil

}

// func markForUpdatingAssoc(db *gorm.DB, v reflect.Value, car cargo) error {
// 	for i := 0; i < v.NumField(); i++ {
// 		t := pegPegassocOrPegManyToMany(v.Type().Field(i).Tag)
// 		if t == "pegassoc" {
// 			switch v.Field(i).Kind() {
// 			case reflect.Struct:
// 				m := v.Field(i).Addr().Interface().(mdl.IModel)
// 				fieldTableName := mdl.GetTableNameFromIModel(m)
// 				if _, ok := car.toProcess[fieldTableName]; ok {
// 					mids := car.toProcess[fieldTableName]
// 					mids.ids = append(mids.ids, m.GetID())
// 					car.toProcess[fieldTableName] = mids
// 				} else {
// 					arr := make([]interface{}, 1)
// 					arr[0] = m.GetID()
// 					car.toProcess[fieldTableName] = modelAndIds{modelObj: m, ids: arr}
// 				}
// 			case reflect.Slice:
// 				typ := v.Type().Field(i).Type.Elem()
// 				m, _ := reflect.New(typ).Interface().(mdl.IModel)
// 				fieldTableName := mdl.GetTableNameFromIModel(m)
// 				for j := 0; j < v.Field(i).Len(); j++ {
// 					if _, ok := car.toProcess[fieldTableName]; ok {
// 						mids := car.toProcess[fieldTableName]
// 						mids.ids = append(mids.ids, v.Field(i).Index(j).Addr().Interface().(mdl.IModel).GetID())
// 						car.toProcess[fieldTableName] = mids
// 					} else {
// 						arr := make([]interface{}, 1)
// 						arr[0] = v.Field(i).Index(j).Addr().Interface().(mdl.IModel).GetID()
// 						car.toProcess[fieldTableName] = modelAndIds{modelObj: m, ids: arr}
// 					}
// 				}
// 			case reflect.Ptr:
// 				// Unbox the pointer
// 				if err := markForUpdatingAssoc(db, v.Elem(), car); err != nil {
// 					return err
// 				}
// 			}
// 		}
// 	}

// 	return nil
// }

// TODO: if there is a "pegassoc-manytomany" inside a pegged struct
// and we're deleting the pegged struct, the many-to-many relationship needs to be removed
func markForDelete(db *gorm.DB, v reflect.Value, car cargo) error {
	for i := 0; i < v.NumField(); i++ {
		t := pegPegassocOrPegManyToMany(v.Type().Field(i).Tag)
		if t == "peg" {
			switch v.Field(i).Kind() {
			case reflect.Struct:
				m := v.Field(i).Addr().Interface().(mdl.IModel)
				if m.GetID() != nil { // could be embedded struct that never get initialiezd
					fieldTableName := mdl.GetTableNameFromIModel(m)
					if _, ok := car.toProcess[fieldTableName]; ok {
						mids := car.toProcess[fieldTableName]
						mids.ids = append(mids.ids, m.GetID())
						car.toProcess[fieldTableName] = mids
					} else {
						arr := make([]interface{}, 1)
						arr[0] = m.GetID()
						car.toProcess[fieldTableName] = modelAndIds{modelObj: m, ids: arr}
					}

					// Traverse into it
					if err := markForDelete(db, v.Field(i), car); err != nil {
						return err
					}
				}
			case reflect.Slice:
				typ := v.Type().Field(i).Type.Elem()
				m, _ := reflect.New(typ).Interface().(mdl.IModel)
				fieldTableName := mdl.GetTableNameFromIModel(m)
				for j := 0; j < v.Field(i).Len(); j++ {
					if _, ok := car.toProcess[fieldTableName]; ok {
						mids := car.toProcess[fieldTableName]
						mids.ids = append(mids.ids, v.Field(i).Index(j).Addr().Interface().(mdl.IModel).GetID())
						car.toProcess[fieldTableName] = mids
					} else {
						arr := make([]interface{}, 1)
						arr[0] = v.Field(i).Index(j).Addr().Interface().(mdl.IModel).GetID()
						car.toProcess[fieldTableName] = modelAndIds{modelObj: m, ids: arr}
					}

					// Can it be a pointer type inside?, then unbox it in the next recursion
					if err := markForDelete(db, v.Field(i).Index(j), car); err != nil {
						return err
					}
				}
			case reflect.Ptr:
				// Need to dereference and get the struct id before traversing in
				if !isNil(v.Field(i)) && !isNil(v.Field(i).Elem()) &&
					v.Field(i).IsValid() && v.Field(i).Elem().IsValid() {
					imodel := v.Field(i).Interface().(mdl.IModel)
					fieldTableName := mdl.GetTableNameFromIModel(imodel)
					id := v.Field(i).Interface().(mdl.IModel).GetID()

					if _, ok := car.toProcess[fieldTableName]; ok {
						mids := car.toProcess[fieldTableName]
						mids.ids = append(mids.ids, id)
						car.toProcess[fieldTableName] = mids
					} else {
						arr := make([]interface{}, 1)
						arr[0] = id
						car.toProcess[fieldTableName] = modelAndIds{modelObj: imodel, ids: arr}
					}

					if err := markForDelete(db, v.Field(i).Elem(), car); err != nil {
						return err
					}
				}
			}
		} else if strings.HasPrefix(t, "pegassoc-manytomany") {
			// We're deleting. And now we have a many to many in here
			// Remove the many to many
			var m mdl.IModel
			switch v.Field(i).Kind() {
			case reflect.Struct:
				m = v.Field(i).Addr().Interface().(mdl.IModel)
			case reflect.Slice:
				typ := v.Type().Field(i).Type.Elem()
				m = reflect.New(typ).Interface().(mdl.IModel)
			case reflect.Ptr:
				m = v.Elem().Interface().(mdl.IModel)
			}
			if err := removeManyToManyAssociationTableElem(db, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeManyToManyAssociationTableElem(db *gorm.DB, modelObj mdl.IModel) error {
	// many to many, here we remove the entry in the actual immediate table
	// because that's actually the link table. Thought we don't delete the
	// Model table itself
	v := reflect.Indirect(reflect.ValueOf(modelObj))
	for i := 0; i < v.NumField(); i++ {
		tag := v.Type().Field(i).Tag.Get("betterrest")
		if strings.HasPrefix(tag, "pegassoc-manytomany") {
			// many to many, here we remove the entry in the actual immediate table
			// because that's actually the link table. Thought we don't delete the
			// Model table itself

			// The normal Delete(mdl, ids) doesn't quite work because
			// I don't have access to the mdl, it's not registered as typestring
			// nor part of the field type. It's a joining table between many to many

			linkTableName := strings.Split(tag, ":")[1]
			typ := v.Type().Field(i).Type.Elem() // Get the type of the element of slice
			m2, _ := reflect.New(typ).Interface().(mdl.IModel)
			fieldTableName := mdl.GetTableNameFromIModel(m2)
			selfTableName := mdl.GetTableNameFromIModel(modelObj)

			fieldVal := v.Field(i)
			if fieldVal.Len() >= 1 {
				uuidStmts := strings.Repeat("?,", fieldVal.Len())
				uuidStmts = uuidStmts[:len(uuidStmts)-1]

				allIds := make([]interface{}, 0, 10)
				allIds = append(allIds, modelObj.GetID().String())
				for j := 0; j < fieldVal.Len(); j++ {
					idToDel := fieldVal.Index(j).FieldByName("ID").Interface().(*datatype.UUID)
					allIds = append(allIds, idToDel.String())
				}

				stmt := fmt.Sprintf("DELETE FROM \"%s\" WHERE \"%s\" = ? AND \"%s\" IN (%s)",
					linkTableName, selfTableName+"_id", fieldTableName+"_id", uuidStmts)
				err := db.Exec(stmt, allIds...).Error
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func pegPegassocOrPegManyToMany(tag reflect.StructTag) string {
	for _, tag := range strings.Split(tag.Get("betterrest"), ";") {
		if tag == "peg" || tag == "pegassoc" || strings.HasPrefix(tag, "pegassoc-manytomany") {
			return tag
		}
	}
	return ""
}

func checkIDsNotFound(db *gorm.DB, nestedIModels []mdl.IModel) error {
	if len(nestedIModels) > 0 {
		ids := make([]*datatype.UUID, 0)
		for _, nestedIModel := range nestedIModels {
			id := nestedIModel.GetID()
			if id != nil {
				ids = append(ids, id)
			}
		}

		if len(ids) == 0 {
			// nothing to check
			return nil
		}

		tableName := mdl.GetTableNameFromIModel(nestedIModels[0])
		var count int64
		err := db.Table(tableName).Where("id IN (?)", ids).Count(&count).Error
		if err != nil && !gorm.IsRecordNotFoundError(err) {
			return err // some real error
		}
		if count != 0 {
			return fmt.Errorf("id of embedded pegged object already exists")
		}
	}

	return nil
}

func RemoveIDForNonPegOrPeggedFieldsBeforeCreate(db *gorm.DB, modelObj mdl.IModel) error {
	v := reflect.Indirect(reflect.ValueOf(modelObj))

	for i := 0; i < v.NumField(); i++ {
		tag := v.Type().Field(i).Tag.Get("betterrest")
		if tag == "peg" {
			// if it's pegged and it's creating, it should be new ID, so we set it nil..
			// or at least it shouldn't exists! (TODO)
			// what if it's the third level?
			fieldVal := v.Field(i)
			switch fieldVal.Kind() {
			case reflect.Slice:
				// Loop through the slice
				ms := make([]mdl.IModel, 0)
				for j := 0; j < fieldVal.Len(); j++ {
					nestedModel := fieldVal.Index(j).Addr().Interface()
					nestedIModel, ok := nestedModel.(mdl.IModel)
					if ok {
						ms = append(ms, nestedIModel)
					}
				}

				if err := checkIDsNotFound(db, ms); err != nil {
					return err
				}

				for j := 0; j < len(ms); j++ {
					nestedIModel := ms[j]
					// Traverse into it
					if err := RemoveIDForNonPegOrPeggedFieldsBeforeCreate(db, nestedIModel); err != nil {
						return err
					}
				}
			case reflect.Ptr:
				nestedModel := v.Field(i).Interface()
				nestedIModel, ok := nestedModel.(mdl.IModel)
				if ok && !isNil(nestedIModel) {
					if nestedIModel.GetID() != nil {
						if nestedIModel.GetID() != nil {
							if err := checkIDsNotFound(db, []mdl.IModel{nestedIModel}); err != nil {
								return err
							}
						}
					}
					// Traverse into it
					if err := RemoveIDForNonPegOrPeggedFieldsBeforeCreate(db, nestedIModel); err != nil {
						return err
					}
				}

			case reflect.Struct:
				nestedModel := v.Field(i).Addr().Interface()
				nestedIModel, ok := nestedModel.(mdl.IModel)
				if ok {
					if nestedIModel.GetID() != nil {
						if err := checkIDsNotFound(db, []mdl.IModel{nestedIModel}); err != nil {
							return err
						}
					}

					// Traverse into it
					if err := RemoveIDForNonPegOrPeggedFieldsBeforeCreate(db, nestedIModel); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// CreatePeggedAssocFields :-
func CreatePeggedAssocFields(db *gorm.DB, modelObj mdl.IModel) (err error) {
	v := reflect.Indirect(reflect.ValueOf(modelObj))
	for i := 0; i < v.NumField(); i++ {
		tag := v.Type().Field(i).Tag.Get("betterrest")
		// columnName := v.Type().Field(i).Name
		if tag == "pegassoc" {
			fieldVal := v.Field(i)
			switch fieldVal.Kind() {
			case reflect.Slice:
				// Loop through the slice
				for j := 0; j < fieldVal.Len(); j++ {
					// nestedModelID := fieldVal.Index(j).FieldByName("ID").Interface().(*datatype.UUID)
					nestedModel := fieldVal.Index(j).Addr().Interface()
					nestedIModel, ok := nestedModel.(mdl.IModel)
					if ok && nestedIModel.GetID() != nil {
						tableName := mdl.GetTableNameFromIModel(modelObj)
						correspondingColumnName := tableName + "_id"
						// Where clause is not needed when the embedded is a struct, but if it's a pointer to struct then it's needed
						if err := db.Model(nestedModel).Where("id = ?", nestedModel.(mdl.IModel).GetID()).
							Update(correspondingColumnName, modelObj.GetID()).Error; err != nil {
							return err
						}
					}

					// // this loops forever unlike update, why?
					// if err = db.Set("gorm:association_autoupdate", true).Model(modelObj).Association(columnName).Append(nestedModel).Error; err != nil {
					// 	return err
					// }
				}
			case reflect.Ptr:
				nestedModel := v.Field(i).Interface()
				nestedIModel, ok := nestedModel.(mdl.IModel)
				if ok && !isNil(nestedIModel) && nestedIModel.GetID() != nil {
					tableName := mdl.GetTableNameFromIModel(modelObj)
					correspondingColumnName := tableName + "_id"
					// Where clause is not needed when the embedded is a struct, but if it's a pointer to struct then it's needed
					if err := db.Model(nestedModel).Where("id = ?", nestedModel.(mdl.IModel).GetID()).Update(correspondingColumnName, modelObj.GetID()).Error; err != nil {
						return err
					}
				}
			case reflect.Struct:
				nestedModel := v.Field(i).Addr().Interface()
				nestedIModel, ok := nestedModel.(mdl.IModel)
				if ok && nestedIModel.GetID() != nil {
					tableName := mdl.GetTableNameFromIModel(modelObj)
					correspondingColumnName := tableName + "_id"
					// Where clause is not needed when the embedded is a struct, but if it's a pointer to struct then it's needed
					if err := db.Model(nestedModel).Where("id = ?", nestedModel.(mdl.IModel).GetID()).
						Update(correspondingColumnName, modelObj.GetID()).Error; err != nil {
						return err
					}
				}
			default:
				// embedded object is considered part of the structure, so no removal
			}
		}
	}
	return nil
}

func isNil(a interface{}) bool {
	defer func() { recover() }()
	return a == nil || reflect.ValueOf(a).IsNil()
}
